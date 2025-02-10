package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	AVALANCHE_NETWORK  = "avalanche"
	AVALANCHE_CHAIN_ID = 43114
)

type AvaxTxsResponse struct {
	// not exactly the same as the Etherscan response but works for gas calculations
	Items []AvaxEVMTxDetails `json:"items"`
	// // don't need this
	// Links map[string]interface{} `json:"links"`
}

type AvaxEVMTxDetails struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	ChainID     string `json:"chainId"`
	Timestamp   string `json:"timestamp"`
	BlockNumber int64  `json:"blockNumber"`
	BlockHash   string `json:"blockHash"`
	From        string `json:"from"`
	To          string `json:"to"`
	Value       string `json:"value"`
	Index       int64  `json:"index"`
	Status      bool   `json:"status"`
	GasUsed     string `json:"gasUsed"`
	GasPrice    string `json:"gasPrice"`
	GasLimit    string `json:"gasLimit"`
	BurnedFees  string `json:"burnedFees"`

	Network    string `json:"network,omitempty"` // not in the response -- injected by us
	GasUsedUsd string `json:"gasUsedUsd"`        // not in the response -- calculated by us
}

func (td *AvaxEVMTxDetails) ToEthTxDetails() EthTxDetails {
	// 2025-02-06T21:03:39.000Z
	useTs := time.Unix(0, 0)
	parsed, err := time.Parse(time.RFC3339, td.Timestamp)
	if err == nil {
		useTs = parsed
	}

	return EthTxDetails{
		Hash:             td.ID,
		BlockNumber:      strconv.FormatInt(td.BlockNumber, 10),
		BlockHash:        td.BlockHash,
		TimeStamp:        strconv.FormatInt(useTs.Unix(), 10),
		GasUsed:          td.GasUsed,
		GasPrice:         td.GasPrice,
		GasPriceBid:      td.GasPrice,
		TransactionIndex: strconv.FormatInt(td.Index, 10),
		From:             td.From,
		To:               td.To,
		Value:            td.Value,
		IsError:          strconv.FormatBool(!td.Status),

		GasUsedUsd: td.GasUsedUsd,
		Network:    AVALANCHE_NETWORK,
	}
}

type AvaxAddressesResponse struct {
	Address               string    `json:"address"`
	Balance               string    `json:"balance"` // 18 decimals
	FirstActivity         time.Time `json:"firstActivity"`
	TransactionsCount     int       `json:"transactionsCount"`
	ERC20TransfersCount   int       `json:"erc20TransfersCount"`
	ERC721TransfersCount  int       `json:"erc721TransfersCount"`
	ERC1155TransfersCount int       `json:"erc1155TransfersCount"`
}

type AvaxErc20HoldingResponse struct {
	Items []AvaxErc20 `json:"items"`
}

type AvaxErc20 struct {
	ChainId         string `json:"chainId"`
	TokenAddress    string `json:"tokenAddress"`
	TokenName       string `json:"tokenName"`
	TokenSymbol     string `json:"tokenSymbol"`
	TokenDecimals   int    `json:"tokenDecimals"`
	TokenQuantity   string `json:"tokenQuantity"`
	TokenPrice      string `json:"tokenPrice"`
	TokenValueInUsd string `json:"tokenValueInUsd"`
	UpdatedAtBlock  int64  `json:"updatedAtBlock"`
}

func (m *Monitor) RunAvalancheBalances() {
	apiUrl := m.cfg.Avalanche.ApiUrl
	address := m.cfg.Avalanche.Address
	useTs := time.Now()

	avaxWei, err := m.getAvaxGasBalance(apiUrl, address)
	if err != nil {
		m.logger.Error().Err(err).
			Str("address", address).
			Str("network", AVALANCHE_NETWORK).
			Msg("failed to get AVAX balance")
	}

	if avaxWei != "" {
		avaxBalance := DbBalance{
			Timestamp: useTs.Unix(),
			Balance:   avaxWei,
			Exponent:  18,
			Token:     "AVAX",
			Address:   address,
			Network:   AVALANCHE_NETWORK,
		}

		m.logger.Debug().Str("network", AVALANCHE_NETWORK).Msg("inserting AVAX balance")
		if err := m.InsertBalance(avaxBalance); err != nil {
			m.logger.Error().Err(err).Str("network", AVALANCHE_NETWORK).Msg("failed to insert balance")
		}

		if avaxDecimal, err := decimal.NewFromString(avaxWei); err == nil {
			m.logger.Info().
				Str("AVAX", avaxDecimal.Shift(-18).String()).
				Str("network", AVALANCHE_NETWORK).
				Str("datetime", useTs.Format(time.RFC3339)).
				Msg("current balance")
		}
	}

	// sleep to avoid rate limiting -> 2 requests per second for free tier
	time.Sleep(1 * time.Second)
	usdc, err := m.getAvaxUSDCBalance(apiUrl, address)
	if err != nil {
		m.logger.Warn().Err(err).
			Str("address", address).
			Str("network", AVALANCHE_NETWORK).
			Msg("failed to get USDC balance")
	}

	if usdc != "" {
		usdcBalance := DbBalance{
			Timestamp: useTs.Unix(),
			Balance:   usdc,
			Exponent:  6,
			Token:     "USDC",
			Address:   address,
			Network:   AVALANCHE_NETWORK,
		}
		if err := m.InsertBalance(usdcBalance); err != nil {
			m.logger.Error().Err(err).Str("network", AVALANCHE_NETWORK).Msg("failed to insert balance")
		}
		if usdcDecimal, err := decimal.NewFromString(usdc); err == nil {
			m.logger.Info().
				Str("USDC", usdcDecimal.Shift(-6).String()).
				Str("network", AVALANCHE_NETWORK).
				Str("datetime", useTs.Format(time.RFC3339)).
				Msg("current balance")
		}
	}
}

func (m *Monitor) RunAvalancheTxHistory(saveRawResponses bool) {
	apiUrl := m.cfg.Avalanche.ApiUrl
	address := m.cfg.Avalanche.Address

	txs, err := m.getAvaxTxs(apiUrl, address)
	if err != nil {
		m.logger.Error().Err(err).Msg("failed to get avalanche txs")
		return
	}
	latestHeight, err := m.GetLatestEthHeight(AVALANCHE_NETWORK)
	if err != nil {
		m.logger.Warn().Msg("failed to get latest avalanche height -- starting from 0")
	}

	priceUsd, err := m.GetLatestUsdTokenPriceDecimal(COINGECKO_AVALANCHE_ID)
	if err != nil {
		m.logger.Error().Err(err).Msg("failed to get latest USD token price")
		return
	}

	inserted := 0
	failed := 0
	totalGasUsedUsd := decimal.NewFromInt(0)
	for _, tx := range txs {
		height, err := strconv.ParseInt(tx.BlockNumber, 10, 64)
		if err != nil {
			m.logger.Error().Err(err).Msg("failed to parse block number")
			continue
		}
		if height <= latestHeight {
			continue
		}

		// just report the error if it happens
		// this will return zero decimal if there is an error so it's ok
		gasUsedUsd, err := calculateGasUSD(priceUsd, tx.GasUsed, tx.GasPrice)
		if err != nil {
			m.logger.Error().Err(err).
				Str("tx_hash", tx.Hash).
				Str("block_number", tx.BlockNumber).
				Str("network", AVALANCHE_NETWORK).
				Msg("failed to calculate gas used USD")
		}

		totalGasUsedUsd = totalGasUsedUsd.Add(gasUsedUsd)
		tx.GasUsedUsd = gasUsedUsd.String()
		tx.Network = AVALANCHE_NETWORK
		if err := m.InsertEthTxResponse(tx, AVALANCHE_NETWORK, saveRawResponses); err != nil {
			m.logger.Error().Err(err).
				Str("tx_hash", tx.Hash).
				Str("block_number", tx.BlockNumber).
				Str("network", AVALANCHE_NETWORK).
				Msg("failed to insert avalanche tx")
			failed++
			continue
		}
		inserted++
	}

	totalGasUsed := m.getGasUsedForTxs(txs)
	m.logger.Info().Int("total", len(txs)).
		Int("new", inserted).
		Int("failed", failed).
		Str("total_gas_used_avax", decimal.NewFromBigInt(totalGasUsed, -18).String()).
		Str("total_gas_used_usd", totalGasUsedUsd.String()).
		Msg("finished processing AVALANCHE txs history")
}

func (m *Monitor) getAvaxTxs(apiUrl string, address string) ([]EthTxDetails, error) {
	headers := map[string]string{"Accept": "application/json"}

	params := url.Values{}
	params.Add("sort", "desc")
	params.Add("limit", "25")

	path := fmt.Sprintf("%s/address/%s/transactions", apiUrl, address)
	url := fmt.Sprintf("%s?%s", path, params.Encode())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data AvaxTxsResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	m.logger.Info().Int("total", len(data.Items)).Str("source", apiUrl).Msg("fetched txs")
	txs := make([]EthTxDetails, len(data.Items))
	for i, tx := range data.Items {
		txs[i] = tx.ToEthTxDetails()
	}
	return txs, nil
}

func (m *Monitor) getAvaxGasBalance(apiUrl string, address string) (string, error) {
	headers := map[string]string{"Accept": "application/json"}

	params := url.Values{}
	params.Add("sort", "desc")

	path := fmt.Sprintf("%s/addresses/%s", apiUrl, address)
	url := fmt.Sprintf("%s?%s", path, params.Encode())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var data AvaxAddressesResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}

	return data.Balance, nil
}

func (m *Monitor) getAvaxUSDCBalance(apiUrl string, address string) (string, error) {
	usdcAddress := m.cfg.Avalanche.UsdcAddress
	headers := map[string]string{"Accept": "application/json"}

	params := url.Values{}
	params.Add("sort", "desc")

	path := fmt.Sprintf("%s/address/%s/erc20-holdings", apiUrl, address)
	url := fmt.Sprintf("%s?%s", path, params.Encode())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var data AvaxErc20HoldingResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}

	for _, item := range data.Items {
		if strings.ToLower(item.TokenAddress) == strings.ToLower(usdcAddress) {
			return item.TokenQuantity, nil
		}
	}

	return "", fmt.Errorf("USDC balance not found")
}
