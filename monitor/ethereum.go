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

	"github.com/shopspring/decimal"
)

const DEFAULT_ARBITRUM_API_URL = "https://api.arbiscan.io/api"

const (
	ARBITRUM_NETWORK  = "arbitrum"
	ARBITRUM_CHAIN_ID = 42161

	ETHEREUM_NETWORK  = "ethereum"
	ETHEREUM_CHAIN_ID = 1
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
		m.logger.Warn().Err(err).Msg("failed to get latest arbitrum height")
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
		m.logger.Warn().Err(err).Msg("failed to get latest ethereum height")
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
