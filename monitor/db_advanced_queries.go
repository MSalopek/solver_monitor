package monitor

import "fmt"

type OrderDetailsInRange struct {
	AmountRange           string  `json:"amount_range"`
	TotalOrdersInRange    int     `json:"total_orders_in_range"`
	ExecutedOrdersInRange int     `json:"executed_orders_in_range"`
	Filler                string  `json:"filler"`
	TotalVolume           float64 `json:"total_volume"`
	AvgTransfer           float64 `json:"avg_transfer"`
	MinTransfer           float64 `json:"min_transfer"`
	MaxTransfer           float64 `json:"max_transfer"`
	SourceDomain          uint32  `json:"source_domain"`
}

func (m *Monitor) GetOrderDetailsByRange(network string, startBlock uint64, filler string) ([]OrderDetailsInRange, error) {
	useChainId, ok := NetworkToChainId[network]
	if !ok {
		return []OrderDetailsInRange{}, fmt.Errorf("invalid network: %s", network)
	}
	rows, err := m.db.Query(`
WITH ranges AS (
  SELECT '1-250' as range, 1 as min_val, 250000000 as max_val
  UNION SELECT '250-500', 250000000, 500000000
  UNION SELECT '500-1000', 500000000, 1000000000
  UNION SELECT '1000-2000', 1000000000, 2000000000
  UNION SELECT '2000-5000', 2000000000, 5000000000
  UNION SELECT '5000-10000', 5000000000, 10000000000
),
range_totals AS (
  SELECT
    r.range,
    COUNT(*) as total_orders_in_range
  FROM tx_data t
  JOIN ranges r ON t.amount_in >= r.min_val AND t.amount_in < r.max_val
  WHERE source_domain = ? and height >= ?
  GROUP BY r.range
)
SELECT
  r.range as amount_range,
  rt.total_orders_in_range,
  COUNT(*) as executed_orders_in_range,
  t.filler,
  SUM(amount_in)/1000000 as total_volume,
  AVG(amount_in)/1000000 as avg_transfer,
  MIN(amount_in)/1000000 as min_transfer,
  MAX(amount_in)/1000000 as max_transfer,
  source_domain
FROM tx_data t
JOIN ranges r ON t.amount_in >= r.min_val AND t.amount_in < r.max_val
JOIN range_totals rt ON rt.range = r.range
WHERE source_domain = ? and height >= ?
GROUP BY
  r.range,
  rt.total_orders_in_range,
  t.filler,
  source_domain
ORDER BY
  r.min_val,
  t.filler,
  source_domain;
	`, useChainId, startBlock, useChainId, startBlock)
	if err != nil {
		return []OrderDetailsInRange{}, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	orderDetails := []OrderDetailsInRange{}
	for rows.Next() {
		var orderDetail OrderDetailsInRange
		err := rows.Scan(
			&orderDetail.AmountRange,
			&orderDetail.TotalOrdersInRange,
			&orderDetail.ExecutedOrdersInRange,
			&orderDetail.Filler,
			&orderDetail.TotalVolume,
			&orderDetail.AvgTransfer,
			&orderDetail.MinTransfer,
			&orderDetail.MaxTransfer,
			&orderDetail.SourceDomain)
		if err != nil {
			return orderDetails, fmt.Errorf("scan error: %w", err)
		}
		orderDetails = append(orderDetails, orderDetail)
	}

	if filler != "" {
		filtered := []OrderDetailsInRange{}
		for _, order := range orderDetails {
			if order.Filler == filler {
				filtered = append(filtered, order)
			}
		}
		return filtered, nil
	}

	return orderDetails, nil
}
