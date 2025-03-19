package monitor

import (
	_ "github.com/mattn/go-sqlite3"
)

func (m *Monitor) LoadFromFile(path string, saveRawResponses bool) {
	m.logger.Info().Str("file", path).Msg("loading orders from file")
	orders, responses, err := m.OrdersFromFile(path)
	if err != nil {
		m.logger.Error().Err(err).Msg("failed to load orders from file")
		return
	}
	minHeight, maxHeight := int64(0), int64(0)
	latestHeight := m.GetLatestHeight()
	if len(orders) > 0 {
		minHeight, maxHeight = getMinMaxHeight(orders)
		m.logger.Info().
			Int("count", len(orders)).
			Int64("min_height", minHeight).
			Int64("max_height", maxHeight).
			Msg("loaded orders from file")
	} else {
		m.logger.Info().Msg("no orders in file")
		return
	}
	for _, o := range orders {
		if int64(latestHeight) >= o.Height {
			m.logger.Debug().
				Str("tx_hash", o.TxHash).
				Int64("height", o.Height).
				Msg("skipping order filled from osmosis")
			continue
		}
		err := m.InsertOrderFilled(o)
		if err != nil {
			m.logger.Error().Err(err).
				Str("tx_hash", o.TxHash).
				Int64("height", o.Height).
				Int64("last_max_height", maxHeight).
				Msg("failed to insert order filled from osmosis")
			continue
		}
	}
	if saveRawResponses {
		for _, r := range responses {
			m.InsertRawTxResponse(*r)
		}
	}
	m.logger.Info().Int("orders", len(orders)).Int("responses", len(responses)).Msg("wrote orders from file")
}

func (m *Monitor) LoadMissingOrderFromFile(path string) {
	m.logger.Info().Str("file", path).Msg("loading missing orders from file")

	// get all tx_hashes from the db
	rows, err := m.db.Query("SELECT DISTINCT(tx_hash) FROM tx_data")
	if err != nil {
		m.logger.Error().Err(err).Msg("failed to get all tx_hashes from the db")
		return
	}
	defer rows.Close()

	txHashes := map[string]bool{}
	for rows.Next() {
		var txHash string
		err = rows.Scan(&txHash)
		if err != nil {
			m.logger.Error().Err(err).Msg("failed to scan tx_hash")
			continue
		}
		txHashes[txHash] = true
	}

	orders, _, err := m.OrdersFromFile(path)
	if err != nil {
		m.logger.Error().Err(err).Msg("failed to load orders from file")
		return
	}

	inserted := 0
	for _, o := range orders {
		if _, ok := txHashes[o.TxHash]; !ok {
			m.logger.Info().
				Str("tx_hash", o.TxHash).
				Int64("height", o.Height).
				Msg("inserting order filled from osmosis")
			err := m.InsertOrderFilled(o)
			if err != nil {
				m.logger.Error().Err(err).
					Str("tx_hash", o.TxHash).
					Int64("height", o.Height).
					Msg("failed to insert order filled from osmosis")
			}
			inserted++
		}
	}
	m.logger.Info().Int("orders", len(orders)).Int("inserted", inserted).Msg("wrote missing orders from file")
}
