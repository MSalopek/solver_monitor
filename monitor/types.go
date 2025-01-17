package monitor

import "time"

type FillOrderEnvelope struct {
	FillOrder *OrderEnvelope `json:"fill_order"`
}

type OrderEnvelope struct {
	Order  *FastTransferOrder `json:"order"`
	Filler string             `json:"filler"`
}

type FastTransferOrder struct {
	Sender            string `json:"sender"`
	Recipient         string `json:"recipient"`
	AmountIn          string `json:"amount_in"`
	AmountOut         string `json:"amount_out"`
	Nonce             uint32 `json:"nonce"`
	SourceDomain      uint32 `json:"source_domain"`
	DestinationDomain uint32 `json:"destination_domain"`
	TimeoutTimestamp  uint64 `json:"timeout_timestamp"`
	Data              string `json:"data,omitempty"`
}

// enrich with db fields
type OrderWithMeta struct {
	FastTransferOrder
	Code               int       `json:"code"`
	TxHash             string    `json:"tx_hash"`
	Height             int       `json:"height"`
	SolverRevenue      int       `json:"solver_revenue"`
	IngestionTimestamp time.Time `json:"ingestion_timestamp"`
}
