package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

const DEFAULT_ARBITRUM_API_URL = "https://api.arbiscan.io/api"

const (
	ARBITRUM_NETWORK  = "arbitrum"
	ARBITRUM_CHAIN_ID = 42161

	ETHEREUM_NETWORK  = "ethereum"
	ETHEREUM_CHAIN_ID = 1

	BASE_NETWORK  = "base"
	BASE_CHAIN_ID = 8453
)

type EthTxDetails struct {
	BlockNumber       string `json:"blockNumber"`
	BlockHash         string `json:"blockHash"`
	TimeStamp         string `json:"timeStamp"`
	Hash              string `json:"hash"`
	Nonce             string `json:"nonce"`
	TransactionIndex  string `json:"transactionIndex"`
	From              string `json:"from"`
	To                string `json:"to"`
	Value             string `json:"value"`
	Gas               string `json:"gas"`
	GasPrice          string `json:"gasPrice"`
	GasPriceBid       string `json:"gasPriceBid"`
	Input             string `json:"input"`
	MethodId          string `json:"methodId"`
	FunctionName      string `json:"functionName"`
	ContractAddress   string `json:"contractAddress"`
	CumulativeGasUsed string `json:"cumulativeGasUsed"`
	TxreceiptStatus   string `json:"txreceipt_status"`
	GasUsed           string `json:"gasUsed"`
	Confirmations     string `json:"confirmations"`
	IsError           string `json:"isError"`
	Network           string `json:"network,omitempty"` // not in the response -- injected by us
}

type EthScanTxListResponse struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Result  []EthTxDetails `json:"result"`
}

type EthBalanceResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Result  string `json:"result"` // token balance -> 18 decimals for ETH, 6 decimals for USDC
}

func (m *Monitor) RunArbitrumTxHistory(saveRawResponses bool) {
	apiUrl := m.cfg.Arbitrum.ApiUrl
	address := m.cfg.Arbitrum.Address
	apiKey := m.cfg.Arbitrum.Key

	txs, err := m.getEthereumTxs(apiUrl, address, apiKey)
	if err != nil {
		m.logger.Error().Err(err).Msg("failed to get arbitrum txs")
		return
	}
	latestHeight, err := m.GetLatestEthHeight(ARBITRUM_NETWORK)
	if err != nil {
		m.logger.Warn().Msg("failed to get latest arbitrum height -- starting from 0")
	}

	inserted := 0
	failed := 0
	for _, tx := range txs {
		height, err := strconv.ParseInt(tx.BlockNumber, 10, 64)
		if err != nil {
			m.logger.Error().Err(err).Msg("failed to parse block number")
			continue
		}
		if height <= latestHeight {
			continue
		}

		tx.Network = ARBITRUM_NETWORK
		if err := m.InsertEthTxResponse(tx, ARBITRUM_NETWORK, saveRawResponses); err != nil {
			m.logger.Error().Err(err).
				Str("tx_hash", tx.Hash).
				Str("block_number", tx.BlockNumber).
				Str("network", ARBITRUM_NETWORK).
				Msg("failed to insert arbitrum tx")
			failed++
			continue
		}
		inserted++
	}

	totalGasUsed := m.getGasUsed(txs)
	m.logger.Info().Int("total", len(txs)).
		Int("new", inserted).
		Int("failed", failed).
		Str("total_gas_used_eth", decimal.NewFromBigInt(totalGasUsed, -18).String()).
		Msg("finished processing ARBITRUM txs history")
}

func (m *Monitor) RunEthereumTxHistory(saveRawResponses bool) {
	apiUrl := m.cfg.Ethereum.ApiUrl
	address := m.cfg.Ethereum.Address
	apiKey := m.cfg.Ethereum.Key

	txs, err := m.getEthereumTxs(apiUrl, address, apiKey)
	if err != nil {
		m.logger.Error().Err(err).Msg("failed to get ethereum txs")
		return
	}
	latestHeight, err := m.GetLatestEthHeight(ETHEREUM_NETWORK)
	if err != nil {
		m.logger.Warn().Msg("failed to get latest ethereum height -- starting from 0")
	}

	inserted := 0
	failed := 0
	for _, tx := range txs {
		height, err := strconv.ParseInt(tx.BlockNumber, 10, 64)
		if err != nil {
			m.logger.Error().Err(err).Msg("failed to parse block number")
			continue
		}
		if height <= latestHeight {
			continue
		}

		tx.Network = ETHEREUM_NETWORK
		if err := m.InsertEthTxResponse(tx, ETHEREUM_NETWORK, saveRawResponses); err != nil {
			m.logger.Error().Err(err).
				Str("tx_hash", tx.Hash).
				Str("block_number", tx.BlockNumber).
				Str("network", ETHEREUM_NETWORK).
				Msg("failed to insert ethereum tx")
			failed++
			continue
		}
		inserted++
	}

	totalGasUsed := m.getGasUsed(txs)
	m.logger.Info().Int("total", len(txs)).
		Int("new", inserted).
		Int("failed", failed).
		Str("total_gas_used_eth", decimal.NewFromBigInt(totalGasUsed, -18).String()).
		Msg("finished processing ETHEREUM txs history")
}

// ethereum balances are handled as strings and stored as strings in the db
// sqlite cannot store 256 bit integers, so we use strings to get around that
func (m *Monitor) RunEthereumBalances() {
	apiUrl := m.cfg.Ethereum.ApiUrl
	address := m.cfg.Ethereum.Address
	apiKey := m.cfg.Ethereum.Key
	useTs := time.Now()

	ethWei, err := m.getEthereumBalance(apiUrl, address, apiKey, "")
	if err != nil {
		m.logger.Error().Err(err).
			Str("address", address).
			Str("network", ETHEREUM_NETWORK).
			Msg("failed to get ETH balance")
	}

	usdc, err := m.getEthereumBalance(apiUrl, address, apiKey, m.cfg.Ethereum.UsdcAddress)
	if err != nil {
		m.logger.Error().Err(err).
			Str("address", address).
			Str("network", ETHEREUM_NETWORK).
			Msg("failed to get USDC balance")
	}

	if ethWei != "" {
		ethBalance := DbBalance{
			Timestamp: useTs.Unix(),
			Balance:   ethWei,
			Exponent:  18,
			Token:     "ETH",
			Address:   address,
			Network:   ETHEREUM_NETWORK,
		}
		if err := m.InsertBalance(ethBalance); err != nil {
			m.logger.Error().Err(err).Str("network", ETHEREUM_NETWORK).Msg("failed to insert balance")
		}

		if ethDecimal, err := decimal.NewFromString(ethWei); err == nil {
			m.logger.Info().
				Str("ETH", ethDecimal.Shift(-18).String()).
				Str("network", ETHEREUM_NETWORK).
				Str("datetime", useTs.Format(time.RFC3339)).
				Msg("current balance")
		}
	}

	if usdc != "" {
		usdcBalance := DbBalance{
			Timestamp: useTs.Unix(),
			Balance:   usdc,
			Exponent:  6,
			Token:     "USDC",
			Address:   address,
			Network:   ETHEREUM_NETWORK,
		}
		if err := m.InsertBalance(usdcBalance); err != nil {
			m.logger.Error().Err(err).Str("network", ETHEREUM_NETWORK).Msg("failed to insert balance")
		}
		if usdcDecimal, err := decimal.NewFromString(usdc); err == nil {
			m.logger.Info().
				Str("USDC", usdcDecimal.Shift(-6).String()).
				Str("network", ETHEREUM_NETWORK).
				Str("datetime", useTs.Format(time.RFC3339)).
				Msg("current balance")
		}
	}
}

func (m *Monitor) RunArbitrumBalances() {
	apiUrl := m.cfg.Arbitrum.ApiUrl
	address := m.cfg.Arbitrum.Address
	apiKey := m.cfg.Arbitrum.Key
	useTs := time.Now()

	ethWei, err := m.getEthereumBalance(apiUrl, address, apiKey, "")
	if err != nil {
		m.logger.Error().Err(err).
			Str("address", address).
			Str("network", ARBITRUM_NETWORK).
			Msg("failed to get ETH balance")
	}

	usdc, err := m.getEthereumBalance(apiUrl, address, apiKey, m.cfg.Arbitrum.UsdcAddress)
	if err != nil {
		m.logger.Error().Err(err).
			Str("address", address).
			Str("network", ARBITRUM_NETWORK).
			Msg("failed to get USDC balance")
	}

	if ethWei != "" {
		ethBalance := DbBalance{
			Timestamp: useTs.Unix(),
			Balance:   ethWei,
			Exponent:  18,
			Token:     "ETH",
			Address:   address,
			Network:   ARBITRUM_NETWORK,
		}
		if err := m.InsertBalance(ethBalance); err != nil {
			m.logger.Error().Err(err).Str("network", ARBITRUM_NETWORK).Msg("failed to insert balance")
		}

		if ethDecimal, err := decimal.NewFromString(ethWei); err == nil {
			m.logger.Info().
				Str("ETH", ethDecimal.Shift(-18).String()).
				Str("network", ARBITRUM_NETWORK).
				Str("datetime", useTs.Format(time.RFC3339)).
				Msg("current balance")
		}
	}

	if usdc != "" {
		usdcBalance := DbBalance{
			Timestamp: useTs.Unix(),
			Balance:   usdc,
			Exponent:  6,
			Token:     "USDC",
			Address:   address,
			Network:   ARBITRUM_NETWORK,
		}
		if err := m.InsertBalance(usdcBalance); err != nil {
			m.logger.Error().Err(err).Str("network", ARBITRUM_NETWORK).Msg("failed to insert balance")
		}
		if usdcDecimal, err := decimal.NewFromString(usdc); err == nil {
			m.logger.Info().
				Str("USDC", usdcDecimal.Shift(-6).String()).
				Str("network", ARBITRUM_NETWORK).
				Str("datetime", useTs.Format(time.RFC3339)).
				Msg("current balance")
		}
	}
}

func (m *Monitor) getGasUsed(txs []EthTxDetails) *big.Int {
	total := new(big.Int)
	for _, tx := range txs {
		gasUsed := new(big.Int)
		gasPrice := new(big.Int)
		if _, success := gasUsed.SetString(tx.GasUsed, 10); !success {
			m.logger.Error().Str("gas_used", tx.GasUsed).Msg("failed to parse gas used")
			continue
		}
		if _, success := gasPrice.SetString(tx.GasPrice, 10); !success {
			m.logger.Error().Str("gas_price", tx.GasPrice).Msg("failed to parse gas price")
			continue
		}
		total.Add(total, gasUsed.Mul(gasUsed, gasPrice))
	}
	return total
}

func (m *Monitor) getEthereumTxs(apiUrl string, address string, apiKey string) ([]EthTxDetails, error) {
	headers := map[string]string{"Accept": "application/json"}

	params := url.Values{}
	params.Add("module", "account")
	params.Add("action", "txlist")
	params.Add("address", address)
	params.Add("startblock", "0")
	params.Add("endblock", "latest")
	params.Add("page", "1")   // fetch all for now - TODO: paginate
	params.Add("offset", "0") // fetch all for now - TODO: paginate
	params.Add("sort", "desc")
	params.Add("apikey", apiKey)

	url := fmt.Sprintf("%s?%s", apiUrl, params.Encode())
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

	var data EthScanTxListResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	return data.Result, nil
}

// getEthereumBalance returns the balance of the given address for the given tokencontract address
// if contract address == "" then it returns the ETH balance in wei
// NOTE:
// * USDC is always 6 decimals
// * ETH is always 18 decimals
// * different L2s use different contract addresses for USDC
func (m *Monitor) getEthereumBalance(apiUrl, address, apiKey, contractAddress string) (string, error) {
	headers := map[string]string{"Accept": "application/json"}

	params := url.Values{}
	params.Add("module", "account")
	params.Add("address", address)
	params.Add("tag", "latest")
	params.Add("apikey", apiKey)

	if contractAddress != "" {
		params.Add("contractaddress", contractAddress)
		params.Add("action", "tokenbalance")
	} else {
		params.Add("action", "balance")
	}

	url := fmt.Sprintf("%s?%s", apiUrl, params.Encode())
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

	var data EthBalanceResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}

	return data.Result, nil

}

func (m *Monitor) GetEthereumTxsFromFile(path string, network string) ([]EthTxDetails, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data EthScanTxListResponse
	if err := json.Unmarshal(file, &data); err != nil {
		return nil, err
	}

	totalGasUsed := m.getGasUsed(data.Result)
	m.logger.Info().Int("total", len(data.Result)).
		Str("total_gas_used_eth", decimal.NewFromBigInt(totalGasUsed, -18).String()).
		Str("network", network).
		Msg("finished processing txs history")

	return data.Result, nil
}
