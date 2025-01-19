package monitor

import (
	"fmt"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

var ChainIdToNetwork = map[string]string{
	"42161":     "arbitrum",
	"43114":     "avalanche",
	"8453":      "base",
	"56":        "bnb",
	"1":         "ethereum",
	"137":       "polygon",
	"osmosis-1": "osmosis",
}

type OrderStatsSummary struct {
	TotalSolverRevenue int64               `json:"total_solver_revenue"`
	TotalOrderCount    int64               `json:"total_order_count"`
	NetworkOrderStats  []NetworkOrderStats `json:"networks"`
}

type NetworkOrderStats struct {
	TotalSolverRevenue int64  `json:"total_solver_revenue"`
	OrderCount         int64  `json:"order_count"`
	Network            string `json:"network"`
}

type FeeStatsSummary struct {
	TotalGasUsed string            `json:"total_gas_used"`
	TotalTxCount int64             `json:"total_tx_count"`
	NetworkStats []NetworkFeeStats `json:"network_stats"`
}

type NetworkFeeStats struct {
	TotalGasUsed string `json:"total_gas_used"`
	TxCount      int64  `json:"tx_count"`
	Network      string `json:"network"`
}

type BalancesByNetworkResponse map[string][]DbBalance

type MaxFillOrderResponse struct {
	TxHash             string    `json:"tx_hash"`
	AmountIn           string    `json:"amount_in"`
	AmountOut          string    `json:"amount_out"`
	Network            string    `json:"network"`
	SolverRevenue      string    `json:"solver_revenue"`
	Height             int64     `json:"height"`
	IngestionTimestamp time.Time `json:"ingestion_timestamp"`
}

type FillStatsResponse struct {
	AverageRevenue   string                 `json:"average_revenue"`
	AverageFill      string                 `json:"average_fill"`
	MaxFill          string                 `json:"max_fill"`
	MaxRevenue       string                 `json:"max_revenue"`
	MinFill          string                 `json:"min_fill"`
	MinRevenue       string                 `json:"min_revenue"`
	MaxFillOrders    []MaxFillOrderResponse `json:"max_fill_details"`
	MaxRevenueOrders []MaxFillOrderResponse `json:"max_revenue_details"`
}

// if useDecimals is true, the balance is returned in decimals
// otherwise, the balance is returned as a string
// this means that for 10^18, the balance will be "1000000000000000000" with useDecimals = false
// and "1" with useDecimals = true
func (m *Monitor) GetDbLatestBalances(network string) ([]DbBalance, error) {
	rows, err := m.db.Query(`
        SELECT address, balance, exponent, token, network, timestamp
        FROM balances
        WHERE (address, token, network, timestamp) IN (
            SELECT address, token, network, MAX(timestamp)
            FROM balances
            GROUP BY address, token, network
        )
    `)
	if err != nil {
		return []DbBalance{}, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	balances := []DbBalance{}
	for rows.Next() {
		var b DbBalance
		err := rows.Scan(&b.Address, &b.Balance, &b.Exponent, &b.Token, &b.Network, &b.Timestamp)
		if err != nil {
			return balances, fmt.Errorf("scan error: %w", err)
		}
		balances = append(balances, b)
	}

	if network != "" {
		balances = []DbBalance{}
		for _, b := range balances {
			if b.Network == network {
				balances = append(balances, b)
			}
		}
	}

	return balances, nil
}

// if useDecimals is true, the balance is returned in decimals
// otherwise, the balance is returned as a string
// this means that for 10^18, the balance will be "1000000000000000000" with useDecimals = false
// and "1" with useDecimals = true
func (m *Monitor) GetDbBalancesInTimeRange(network string, from, to time.Time) ([]DbBalance, error) {
	rows, err := m.db.Query(`
        SELECT address, balance, exponent, token, network, timestamp
        FROM balances
        WHERE network = ? AND timestamp >= ? AND timestamp <= ?
        ORDER BY timestamp DESC
    `, network, from.Unix(), to.Unix())

	if err != nil {
		return []DbBalance{}, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	balances := []DbBalance{}
	for rows.Next() {
		var b DbBalance
		err := rows.Scan(&b.Address, &b.Balance, &b.Exponent, &b.Token, &b.Network, &b.Timestamp)
		if err != nil {
			return balances, fmt.Errorf("scan error: %w", err)
		}
		balances = append(balances, b)
	}

	return balances, nil
}

func (m *Monitor) GetDbFilledOrderStats(filler string) (*OrderStatsSummary, error) {
	if filler == "" {
		return nil, fmt.Errorf("filler address is required")
	}

	rows, err := m.db.Query(`
        SELECT 
            source_domain as network,
            COUNT(*) as order_count,
            SUM(solver_revenue) as total_solver_revenue
        FROM tx_data 
        WHERE filler = ?
        GROUP BY source_domain
    `, filler)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	stats := OrderStatsSummary{}
	for rows.Next() {
		var s NetworkOrderStats
		err := rows.Scan(&s.Network, &s.OrderCount, &s.TotalSolverRevenue)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		stats.NetworkOrderStats = append(stats.NetworkOrderStats, s)
		stats.TotalOrderCount += s.OrderCount
		stats.TotalSolverRevenue += s.TotalSolverRevenue
	}

	return &stats, nil
}

func (m *Monitor) GetDbFeesStats() (*FeeStatsSummary, error) {
	rows, err := m.db.Query(`
        SELECT 
            network,
            COUNT(*) as tx_count,
            SUM(gas_used_wei) as total_gas_used
        FROM eth_tx_responses
        GROUP BY network
    `)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	stats := FeeStatsSummary{}
	totalGasUsedDecimal := decimal.NewFromInt(0)
	for rows.Next() {
		var s NetworkFeeStats
		var txCount int64
		var totalGas int64
		err := rows.Scan(&s.Network, &txCount, &totalGas)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}

		// Convert network chain ID to network name
		if networkName, ok := ChainIdToNetwork[s.Network]; ok {
			s.Network = networkName
		}

		s.TxCount = txCount
		s.TotalGasUsed = strconv.FormatInt(totalGas, 10) // This represents total gas used in wei

		stats.NetworkStats = append(stats.NetworkStats, s)
		stats.TotalTxCount += txCount
		totalGasUsedDecimal = totalGasUsedDecimal.Add(decimal.NewFromInt(totalGas))
	}

	stats.TotalGasUsed = totalGasUsedDecimal.String()

	return &stats, nil
}

func (m *Monitor) ReadMaxAmountInOrdersByFiller(filler string) ([]DbOrderFilled, error) {
	rows, err := m.db.Query(`
		SELECT t1.tx_hash, t1.sender, t1.amount_in, t1.amount_out, t1.source_domain, 
		       t1.solver_revenue, t1.height, t1.code, t1.filler, t1.ingestion_timestamp
		FROM tx_data t1
		INNER JOIN (
			SELECT source_domain, MAX(amount_in) as max_amount_in
			FROM tx_data
			WHERE filler = ?
			GROUP BY source_domain
		) t2 ON t1.source_domain = t2.source_domain 
		AND t1.amount_in = t2.max_amount_in
		WHERE t1.filler = ?
		ORDER BY t1.source_domain, t1.ingestion_timestamp DESC
	`, filler, filler)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var orders []DbOrderFilled
	for rows.Next() {
		var order DbOrderFilled
		err := rows.Scan(
			&order.TxHash, &order.Sender, &order.AmountIn, &order.AmountOut,
			&order.SourceDomain, &order.SolverRevenue, &order.Height, &order.Code,
			&order.Filler, &order.IngestionTimestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		orders = append(orders, order)
	}
	return orders, nil
}

func (m *Monitor) ReadMaxSolverRevenueOrders(filler string) ([]DbOrderFilled, error) {
	rows, err := m.db.Query(`
		SELECT t1.tx_hash, t1.sender, t1.amount_in, t1.amount_out, t1.source_domain, 
		       t1.solver_revenue, t1.height, t1.code, t1.filler, t1.ingestion_timestamp
		FROM tx_data t1
		INNER JOIN (
			SELECT source_domain, MAX(solver_revenue) as max_revenue
			FROM tx_data
			WHERE filler = ?
			GROUP BY source_domain
		) t2 ON t1.source_domain = t2.source_domain 
		AND t1.solver_revenue = t2.max_revenue
		WHERE t1.filler = ?
		ORDER BY t1.source_domain, t1.ingestion_timestamp DESC
	`, filler, filler)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var orders []DbOrderFilled
	for rows.Next() {
		var order DbOrderFilled
		err := rows.Scan(
			&order.TxHash, &order.Sender, &order.AmountIn, &order.AmountOut,
			&order.SourceDomain, &order.SolverRevenue, &order.Height, &order.Code,
			&order.Filler, &order.IngestionTimestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		orders = append(orders, order)
	}
	return orders, nil
}

func (m *Monitor) GetDbAverageRevenue(filler string) (string, error) {
	rows, err := m.db.Query(`
		SELECT AVG(solver_revenue) as average_revenue
		FROM tx_data
		WHERE filler = ?
	`, filler)
	if err != nil {
		return "", fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var averageRevenue string
	if rows.Next() {
		err := rows.Scan(&averageRevenue)
		if err != nil {
			return "", fmt.Errorf("scan error: %w", err)
		}
	}
	return averageRevenue, nil
}

func (m *Monitor) GetDbAverageFill(filler string) (string, error) {
	rows, err := m.db.Query(`
		SELECT AVG(amount_in) as average_fill
		FROM tx_data
		WHERE filler = ?
	`, filler)
	if err != nil {
		return "", fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var averageFill string
	if rows.Next() {
		err := rows.Scan(&averageFill)
		if err != nil {
			return "", fmt.Errorf("scan error: %w", err)
		}
	}
	return averageFill, nil
}

func (m *Monitor) GetDbMinFill(filler string) (string, error) {
	rows, err := m.db.Query(`
		SELECT MIN(amount_in) as min_fill
		FROM tx_data
		WHERE filler = ?
	`, filler)
	if err != nil {
		return "", fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var minFill string
	if rows.Next() {
		err := rows.Scan(&minFill)
		if err != nil {
			return "", fmt.Errorf("scan error: %w", err)
		}
	}
	return minFill, nil
}

func (m *Monitor) GetDbMinRevenue(filler string) (string, error) {
	rows, err := m.db.Query(`
		SELECT MIN(solver_revenue) as min_revenue
		FROM tx_data
		WHERE filler = ?
	`, filler)
	if err != nil {
		return "", fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var minRevenue string
	if rows.Next() {
		err := rows.Scan(&minRevenue)
		if err != nil {
			return "", fmt.Errorf("scan error: %w", err)
		}
	}
	return minRevenue, nil
}

func (m *Monitor) GetDbFillStats(filler string) (*FillStatsResponse, error) {
	maxRev, err := m.ReadMaxSolverRevenueOrders(filler)
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}
	maxFill, err := m.ReadMaxAmountInOrdersByFiller(filler)
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}

	avgRev, err := m.GetDbAverageRevenue(filler)
	if err != nil {
		return nil, fmt.Errorf("failed to get average revenue: %w", err)
	}

	avgRevDecimal, err := decimal.NewFromString(avgRev)
	if err != nil {
		return nil, fmt.Errorf("failed to convert average revenue to decimal: %w", err)
	}

	avgFill, err := m.GetDbAverageFill(filler)
	if err != nil {
		return nil, fmt.Errorf("failed to get average fill: %w", err)
	}
	avgFillDecimal, err := decimal.NewFromString(avgFill)
	if err != nil {
		return nil, fmt.Errorf("failed to convert average fill to decimal: %w", err)
	}

	minFill, err := m.GetDbMinFill(filler)
	if err != nil {
		return nil, fmt.Errorf("failed to get min fill: %w", err)
	}
	minFillDecimal, err := decimal.NewFromString(minFill)
	if err != nil {
		return nil, fmt.Errorf("failed to convert min fill to decimal: %w", err)
	}

	minRevenue, err := m.GetDbMinRevenue(filler)
	if err != nil {
		return nil, fmt.Errorf("failed to get min revenue: %w", err)
	}
	minRevenueDecimal, err := decimal.NewFromString(minRevenue)
	if err != nil {
		return nil, fmt.Errorf("failed to convert min revenue to decimal: %w", err)
	}

	maxFillDecimal, err := decimal.NewFromString("0")
	for _, order := range maxFill {
		amountInDecimal, _ := decimal.NewFromString(order.AmountIn)
		if amountInDecimal.GreaterThan(maxFillDecimal) {
			maxFillDecimal = amountInDecimal
		}
	}

	maxRevDecimal, err := decimal.NewFromString("0")
	for _, order := range maxRev {
		revenueDecimal := decimal.NewFromInt(order.SolverRevenue)
		if revenueDecimal.GreaterThan(maxRevDecimal) {
			maxRevDecimal = revenueDecimal
		}
	}

	response := FillStatsResponse{
		AverageRevenue:   avgRevDecimal.Shift(-6).String(),
		AverageFill:      avgFillDecimal.Shift(-6).String(),
		MaxFill:          maxFillDecimal.Shift(-6).String(),
		MaxRevenue:       maxRevDecimal.Shift(-6).String(),
		MinFill:          minFillDecimal.Shift(-6).String(),
		MinRevenue:       minRevenueDecimal.Shift(-6).String(),
		MaxFillOrders:    []MaxFillOrderResponse{},
		MaxRevenueOrders: []MaxFillOrderResponse{},
	}
	for _, order := range maxFill {
		revenueDecimal := decimal.NewFromInt(order.SolverRevenue).Shift(-6).String()
		amountInDecimal, _ := decimal.NewFromString(order.AmountIn)
		amountOutDecimal, _ := decimal.NewFromString(order.AmountOut)
		chainName, ok := ChainIdToNetwork[order.SourceDomain]
		if !ok {
			chainName = order.SourceDomain
		}
		response.MaxFillOrders = append(response.MaxFillOrders, MaxFillOrderResponse{
			TxHash:             order.TxHash,
			AmountIn:           amountInDecimal.Shift(-6).String(),
			AmountOut:          amountOutDecimal.Shift(-6).String(),
			Network:            chainName,
			SolverRevenue:      revenueDecimal,
			Height:             order.Height,
			IngestionTimestamp: order.IngestionTimestamp,
		})
	}

	for _, order := range maxRev {
		revenueDecimal := decimal.NewFromInt(order.SolverRevenue).Shift(-6).String()
		amountInDecimal, _ := decimal.NewFromString(order.AmountIn)
		amountOutDecimal, _ := decimal.NewFromString(order.AmountOut)
		chainName, ok := ChainIdToNetwork[order.SourceDomain]
		if !ok {
			chainName = order.SourceDomain
		}
		response.MaxRevenueOrders = append(response.MaxRevenueOrders, MaxFillOrderResponse{
			TxHash:             order.TxHash,
			AmountIn:           amountInDecimal.Shift(-6).String(),
			AmountOut:          amountOutDecimal.Shift(-6).String(),
			Network:            chainName,
			SolverRevenue:      revenueDecimal,
			Height:             order.Height,
			IngestionTimestamp: order.IngestionTimestamp,
		})
	}

	return &response, nil
}
