package monitor

import (
	"database/sql"
	"encoding/json"
	"log"
	"math/big"
	"strconv"
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

type DbEthTxResponse struct {
	TxHash     string `json:"tx_hash"`
	Height     int64  `json:"height"`
	Timestamp  int64  `json:"timestamp"`
	GasUsedWei int64  `json:"gas_used_wei"` // value in wei -> gasUsed * gasPrice
	Valid      bool   `json:"valid"`
	Network    string `json:"network"`
	TxResponse []byte `json:"tx_response"` // raw response so we can fallback to local stores if we need to recover or sth
}

type DbBalance struct {
	Timestamp int64  `json:"timestamp"`
	Balance   string `json:"balance"`
	Address   string `json:"address,omitempty"`
	Exponent  int64  `json:"exponent,omitempty"`
	Token     string `json:"token"`
	Network   string `json:"network,omitempty"`
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
		CREATE INDEX IF NOT EXISTS idx_tx_data_filler ON tx_data(filler)
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

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS eth_tx_responses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tx_hash TEXT,
			height INTEGER,
			timestamp INTEGER,
			gas_used_wei INTEGER,
			gas_used_usd REAL,
			network TEXT,
			valid BOOLEAN,
			tx_response TEXT
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS usd_prices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			token_denom TEXT,
			price_usd INTEGER,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS balances (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp INTEGER,
			address TEXT,
			balance TEXT,
			exponent INTEGER,
			token TEXT,
			network TEXT
		)
	`)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS osmo_block_times (
		height INTEGER,
		timestamp INTEGER,
		datetime DATETIME
	)`)

	if err != nil {
		log.Fatal(err)
	}

	// Create timestamp index
	_, err = db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_balances_timestamp 
        ON balances(timestamp)
    `)
	if err != nil {
		log.Fatal(err)
	}

	// Create composite index
	_, err = db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_balances_composite 
        ON balances(address, token, network, timestamp)
    `)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		log.Fatal(err)
	}
}

func (m *Monitor) InsertUsdPrice(denom string, price float64) error {
	query := `INSERT INTO usd_prices (token_denom, price_usd) VALUES (?, ?);`
	_, err := m.db.Exec(query, denom, price)
	return err
}

func (m *Monitor) InsertBalance(balance DbBalance) error {
	_, err := m.db.Exec(`
		INSERT INTO balances (timestamp, balance, exponent, token, network, address)
		VALUES (?, ?, ?, ?, ?, ?)
	`, balance.Timestamp, balance.Balance, balance.Exponent, balance.Token, balance.Network, balance.Address)
	return err
}

func (m *Monitor) InsertRawTxResponse(txResponse DbTxResponse) error {
	_, err := m.db.Exec(`
		INSERT INTO raw_tx_responses (tx_hash, height, tx_response, valid)
		VALUES (?, ?, ?, ?)
	`, txResponse.TxHash, txResponse.Height, txResponse.TxResponse, txResponse.Valid)

	return err
}

func (m *Monitor) InsertEthTxResponse(txResponse EthTxDetails, network string, storeRawResponse bool) error {
	timestamp, err := strconv.Atoi(txResponse.TimeStamp)
	if err != nil {
		return err
	}
	height, err := strconv.Atoi(txResponse.BlockNumber)
	if err != nil {
		return err
	}
	rawResponse := []byte{}
	if storeRawResponse {
		rawResponse, err = json.Marshal(txResponse)
		if err != nil {
			return err
		}
	}
	// gas used is kept as a string because it's a big number (uint256)
	// any calculations will be done in the app (in go code) becasue sqlite doesn't support big numbers
	gasPrice := new(big.Int)
	gasPrice.SetString(txResponse.GasPrice, 10)
	actualGasUsedWei := new(big.Int)
	actualGasUsedWei.SetString(txResponse.GasUsed, 10) // Parse string as base 10
	actualGasUsedWei.Mul(actualGasUsedWei, gasPrice)
	_, err = m.db.Exec(`
		INSERT INTO eth_tx_responses (tx_hash, height, timestamp, gas_used_wei, gas_used_usd, network, valid, tx_response)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, txResponse.Hash, height, timestamp, actualGasUsedWei.String(), txResponse.GasUsedUsd, network, txResponse.IsError, rawResponse)
	return err
}

func (m *Monitor) GetLatestEthHeight(network string) (int64, error) {
	row := m.db.QueryRow(`
		SELECT height FROM eth_tx_responses WHERE network = ? ORDER BY height DESC LIMIT 1
	`, network)
	var height int64
	err := row.Scan(&height)
	if err != nil {
		return 0, err
	}
	return height, nil
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
