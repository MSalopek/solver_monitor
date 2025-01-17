package monitor

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DbOrderFilled struct {
	TxHash             string    `json:"tx_hash"`
	Sender             string    `json:"sender"`
	AmountIn           string    `json:"amount_in"`
	AmountOut          string    `json:"amount_out"`
	SourceDomain       string    `json:"source_domain"`
	SolverRevenue      int64     `json:"solver_revenue"`
	Height             int64     `json:"height"`
	Code               int64     `json:"code"`
	IngestionTimestamp time.Time `json:"ingestion_timestamp"`
	Filler             string    `json:"filler"`
}

type DbTxResponse struct {
	TxHash     string `json:"tx_hash"`
	Height     int64  `json:"height"`
	Valid      bool   `json:"valid"`
	TxResponse []byte `json:"tx_response"`
}

func InitDB(db *sql.DB) {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS tx_data (
			tx_hash TEXT PRIMARY KEY,
			sender TEXT,
			amount_in INTEGER,
			amount_out INTEGER,
			source_domain TEXT,
			solver_revenue INTEGER,
			code INTEGER,
			height INTEGER,
			filler TEXT,
			ingestion_timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS raw_tx_responses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tx_hash TEXT,
			height INTEGER,
			tx_response TEXT,
			valid BOOLEAN
		)
	`)
	if err != nil {
		log.Fatal(err)
	}
}

func (m *Monitor) InsertRawTxResponse(txResponse DbTxResponse) error {
	_, err := m.db.Exec(`
		INSERT INTO raw_tx_responses (tx_hash, height, tx_response, valid)
		VALUES (?, ?, ?, ?)
	`, txResponse.TxHash, txResponse.Height, txResponse.TxResponse, txResponse.Valid)

	return err
}

func (m *Monitor) InsertOrderFilled(order DbOrderFilled) error {
	_, err := m.db.Exec(`
		INSERT INTO tx_data (tx_hash, sender, amount_in, amount_out, source_domain, solver_revenue, height, code, filler, ingestion_timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, order.TxHash, order.Sender, order.AmountIn, order.AmountOut, order.SourceDomain, order.SolverRevenue, order.Height, order.Code, order.Filler, order.IngestionTimestamp)
	if err != nil {
		return err
	}
	return nil
}

func ReadOrdersFilled(db *sql.DB) []DbOrderFilled {
	rows, err := db.Query(`
		SELECT tx_hash, sender, amount_in, amount_out, source_domain, solver_revenue, height, code, filler, ingestion_timestamp
		FROM tx_data
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var orders []DbOrderFilled
	for rows.Next() {
		var order DbOrderFilled
		err := rows.Scan(&order.TxHash, &order.Sender, &order.AmountIn, &order.AmountOut, &order.SourceDomain, &order.SolverRevenue, &order.Height, &order.Code, &order.Filler, &order.IngestionTimestamp)
		if err != nil {
			log.Fatal(err)
		}
		orders = append(orders, order)
	}
	return orders
}

func ReadOrdersByFiller(db *sql.DB, filler string) []DbOrderFilled {
	rows, err := db.Query(`
		SELECT tx_hash, filler, amount_in, amount_out, source_domain, solver_revenue, height, code, ingestion_timestamp
		FROM tx_data
		WHERE filler = ?
	`, filler)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var orders []DbOrderFilled
	for rows.Next() {
		var order DbOrderFilled
		err := rows.Scan(&order.TxHash, &order.Filler, &order.AmountIn, &order.AmountOut, &order.SourceDomain, &order.SolverRevenue, &order.Height, &order.Code, &order.IngestionTimestamp)
		if err != nil {
			log.Fatal(err)
		}
		orders = append(orders, order)
	}
	return orders
}
