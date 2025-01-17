package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/types/tx"
)

func GetAllOsmosisOrders(contract_address string, apiUrl string) {
	allTxs := []interface{}{}
	allTxResponses := []interface{}{}
	headers := map[string]string{"Accept": "application/json"}
	baseURL := fmt.Sprintf("%s/cosmos/tx/v1beta1/txs", apiUrl)
	query := fmt.Sprintf("wasm._contract_address='%s' AND wasm.action='order_filled'", contract_address)

	attempts := 0
	maxAttempts := 20
	timestamp := time.Now().Unix()
	total := 0

	for attempts < maxAttempts {
		params := url.Values{}
		params.Add("limit", "100")
		params.Add("page", strconv.Itoa(attempts+1))
		params.Add("query", query)

		encodedParams := params.Encode()
		fullURL := fmt.Sprintf("%s?%s", baseURL, encodedParams)

		req, err := http.NewRequest("GET", fullURL, nil)
		if err != nil {
			log.Fatal(err)
		}

		for key, value := range headers {
			req.Header.Add(key, value)
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}

		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			log.Fatal(err)
		}

		if totalVal, ok := data["total"].(float64); ok {
			total = int(totalVal)
			fmt.Println(total, "HAVE", len(allTxs))
			if len(allTxs) >= total {
				fmt.Println("COLLECTED ALL")
				break
			}
		}

		if txs, ok := data["txs"].([]interface{}); ok {
			allTxs = append(allTxs, txs...)
		} else {
			break
		}

		if txResponses, ok := data["tx_responses"].([]interface{}); ok {
			allTxResponses = append(allTxResponses, txResponses...)
		}

		filename := fmt.Sprintf("./orders/orders_%d_%d.json", timestamp, attempts)
		attempts++

		fileData := map[string]interface{}{
			"txs":          allTxs,
			"tx_responses": allTxResponses,
		}

		file, err := os.Create(filename)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		encoder := json.NewEncoder(file)
		if err := encoder.Encode(fileData); err != nil {
			log.Fatal(err)
		}
	}
}

func (m *Monitor) GetNewOrders(height int, contractAddress string) ([]DbOrderFilled, []*DbTxResponse, error) {
	headers := map[string]string{"Accept": "application/json"}
	baseURL := fmt.Sprintf("%s/cosmos/tx/v1beta1/txs", m.apiUrl)
	query := fmt.Sprintf("wasm._contract_address='%s' AND wasm.action='order_filled'", contractAddress)

	params := url.Values{}
	params.Add("order_by", "ORDER_BY_DESC")
	params.Add("query", query)

	url := fmt.Sprintf("%s?%s", baseURL, params.Encode())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, err
	}

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	var data tx.GetTxsEventResponse
	if err := m.Codec.UnmarshalJSON(body, &data); err != nil {
		return nil, nil, err
	}

	orders := []DbOrderFilled{}
	responses := []*DbTxResponse{}
	for _, txResponse := range data.TxResponses {
		fillOrders := m.DecodeTxResponse(txResponse)
		for _, fillOrder := range fillOrders {
			amountIn, _ := new(big.Int).SetString(fillOrder.FillOrder.Order.AmountIn, 10)
			amountOut, _ := new(big.Int).SetString(fillOrder.FillOrder.Order.AmountOut, 10)
			revenue := amountIn.Sub(amountIn, amountOut)
			orders = append(orders, DbOrderFilled{
				Code:               int64(txResponse.Code),
				TxHash:             txResponse.TxHash,
				Height:             txResponse.Height,
				Sender:             fillOrder.FillOrder.Order.Sender,
				AmountIn:           fillOrder.FillOrder.Order.AmountIn,
				AmountOut:          fillOrder.FillOrder.Order.AmountOut,
				SourceDomain:       strconv.Itoa(int(fillOrder.FillOrder.Order.SourceDomain)),
				SolverRevenue:      revenue.Int64(),
				IngestionTimestamp: time.Now(),
				Filler:             fillOrder.FillOrder.Filler,
			})
		}

		s, err := m.Codec.MarshalJSON(txResponse)
		if err != nil {
			m.logger.Error().Err(err).Msg("failed to marshal tx response")
			continue
		}
		responses = append(responses, &DbTxResponse{
			TxHash:     txResponse.TxHash,
			Height:     txResponse.Height,
			Valid:      txResponse.Code == 0,
			TxResponse: s,
		})
	}
	return orders, responses, nil
}

type TxsFile struct {
	TxResponses []interface{} `json:"tx_responses"`
	Txs         []interface{} `json:"txs"`
}

func (m *Monitor) OrdersFromFile(filePath string) ([]DbOrderFilled, []*DbTxResponse, error) {
	path := filePath
	if path == "" {
		path = "txs_details.json"
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read messages.json: %w", err)
	}

	var data tx.GetTxsEventResponse
	if err := m.Codec.UnmarshalJSON(body, &data); err != nil {
		m.logger.Error().Err(err).Msg("failed to unmarshal txs_details.json")
		return nil, nil, err
	}

	orders := []DbOrderFilled{}
	responses := []*DbTxResponse{}

	m.logger.Info().Interface("records", len(data.TxResponses)).Msg("found records in file - decoding")
	for _, txResponse := range data.TxResponses {
		fillOrders := m.DecodeTxResponse(txResponse)
		for _, fillOrder := range fillOrders {
			amountIn, _ := new(big.Int).SetString(fillOrder.FillOrder.Order.AmountIn, 10)
			amountOut, _ := new(big.Int).SetString(fillOrder.FillOrder.Order.AmountOut, 10)
			revenue := amountIn.Sub(amountIn, amountOut)
			orders = append(orders, DbOrderFilled{
				Code:               int64(txResponse.Code),
				TxHash:             txResponse.TxHash,
				Height:             txResponse.Height,
				Sender:             fillOrder.FillOrder.Order.Sender,
				AmountIn:           fillOrder.FillOrder.Order.AmountIn,
				AmountOut:          fillOrder.FillOrder.Order.AmountOut,
				SourceDomain:       strconv.Itoa(int(fillOrder.FillOrder.Order.SourceDomain)),
				SolverRevenue:      revenue.Int64(),
				IngestionTimestamp: time.Now(),
				Filler:             fillOrder.FillOrder.Filler,
			})
		}

		s, err := m.Codec.MarshalJSON(txResponse)
		if err != nil {
			m.logger.Error().Err(err).Msg("failed to marshal tx response")
			continue
		}
		responses = append(responses, &DbTxResponse{
			TxHash:     txResponse.TxHash,
			Height:     txResponse.Height,
			Valid:      txResponse.Code == 0,
			TxResponse: s,
		})
	}
	m.logger.Info().Interface("orders", len(orders)).Interface("responses", len(responses)).Msg("decoded orders and responses")
	return orders, responses, nil
}

func (m *Monitor) RunOrders(solverAddress string, contractAddress string, saveRawResponses bool) {
	minHeight, maxHeight := int64(0), int64(0)
	latestHeight := m.GetLatestHeight()
	newOrders, rawResponses, err := m.GetNewOrders(latestHeight, contractAddress)
	if err != nil {
		m.logger.Error().Str("event", "Error fetching new orders").Err(err).Send()
		return
	}

	if len(newOrders) > 0 {
		minHeight, maxHeight = getMinMaxHeight(newOrders)
		logMessage := map[string]string{
			"event":      "transactions_collected",
			"count":      strconv.Itoa(len(newOrders)),
			"min_height": strconv.Itoa(int(minHeight)),
			"max_height": strconv.Itoa(int(maxHeight)),
		}
		m.logger.Info().Interface("details", logMessage).Msg("fetched orders")
	} else {
		m.logger.Info().Msg("no new orders")
	}

	if int64(latestHeight) >= maxHeight {
		m.logger.Info().Msg("no new orders")
		return
	}

	if saveRawResponses {
		for _, tx := range rawResponses {
			if int64(latestHeight) >= tx.Height {
				continue
			}
			err := m.InsertRawTxResponse(*tx)
			if err != nil {
				m.logger.Error().Err(err).Interface("details", map[string]string{
					"tx_hash": tx.TxHash,
					"height":  strconv.Itoa(int(tx.Height)),
				}).Msg("failed to insert raw tx response")
				continue
			}
		}
	}

	saved := 0
	for _, tx := range newOrders {
		if int64(latestHeight) >= tx.Height {
			continue
		}
		err := m.InsertOrderFilled(tx)
		if err != nil {
			m.logger.Error().Err(err).Interface("details", map[string]string{
				"tx_hash":       tx.TxHash,
				"filler":        tx.Filler,
				"amount_in":     tx.AmountIn,
				"amount_out":    tx.AmountOut,
				"source_domain": tx.SourceDomain,
				"height":        strconv.Itoa(int(tx.Height)),
			}).Msg("failed to insert order filled")
			continue
		}
		if solverAddress != "" && tx.Filler == solverAddress {
			m.logger.Info().
				Str("tx_hash", tx.TxHash).
				Int("height", int(tx.Height)).
				Int("revenue", int(tx.SolverRevenue)).
				Msg("monitored solver filled order")
		}
		saved++
	}
	m.logger.Info().Int("count", saved).Msg("saved order fills")
}

func (m *Monitor) GetLatestHeight() int {
	var height int
	err := m.db.QueryRow("SELECT MAX(height) FROM tx_data").Scan(&height)
	if err != nil {
		m.logger.Error().Err(err).Msg("failed to get latest height")
		return 0
	}
	return height
}

func getMinMaxHeight(orders []DbOrderFilled) (int64, int64) {
	if len(orders) == 0 {
		return 0, 0
	}

	hs := []int64{}
	for _, order := range orders {
		hs = append(hs, order.Height)
	}
	sort.Slice(hs, func(i, j int) bool { return hs[i] < hs[j] })

	return hs[0], hs[len(hs)-1]
}
