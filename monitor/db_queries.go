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
