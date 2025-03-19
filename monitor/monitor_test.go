package monitor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

const (
	testDataDir = "testdata"
)

// testFixturePath returns the absolute path to a test fixture file
func testFixturePath(name string) string {
	return filepath.Join(testDataDir, name)
}

// loadTestFixture reads and returns the contents of a test fixture file
func loadTestFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := testFixturePath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read test fixture %s: %v", name, err)
	}
	return data
}

func newTestMonitor() *Monitor {
	logger := zerolog.New(os.Stdout)
	enc := MakeEncodingConfig()
	return &Monitor{
		Codec:             enc.Marshaler,
		interfaceRegistry: enc.InterfaceRegistry,
		amino:             enc.Amino,
		decoder:           MustInitDecoder(),
		logger:            &logger,
	}
}

func TestDecodeTxResponses(t *testing.T) {
	m := newTestMonitor()

	raw := loadTestFixture(t, "decoding_test_fixture.json")

	var data tx.GetTxsEventResponse
	if err := m.Codec.UnmarshalJSON(raw, &data); err != nil {
		t.Fatalf("codec failed to unmarshal test fixture: %v", err)
	}

	txs := []FillOrderEnvelope{}
	for i, txResponse := range data.TxResponses {
		tx := m.DecodeTxResponse(txResponse)
		assert.NotEmpty(t, tx, "tx should not be empty at index %d", i)
		txs = append(txs, tx...)
	}

	assert.Equal(t, 7, len(txs))

	for _, tx := range txs {
		assert.NotZero(t, tx.FillOrder.Order.AmountIn)
		assert.NotZero(t, tx.FillOrder.Order.AmountOut)
		assert.NotZero(t, tx.FillOrder.Order.Nonce)
		assert.NotZero(t, tx.FillOrder.Order.SourceDomain)
		assert.NotZero(t, tx.FillOrder.Order.DestinationDomain)
		assert.NotZero(t, tx.FillOrder.Order.TimeoutTimestamp)
		assert.NotZero(t, tx.FillOrder.Order.Data)
	}

	// check some txs for correctness
	// index 0 tx is wasm.ExecuteContract
	assert.Equal(t, "12000000", txs[0].FillOrder.Order.AmountIn)
	assert.Equal(t, "11928000", txs[0].FillOrder.Order.AmountOut)
	assert.Equal(t, uint32(42161), txs[0].FillOrder.Order.SourceDomain)

	// index 1 should be authz.MsgExec with wasm.ExecuteContract inside
	assert.Equal(t, "1000000", txs[1].FillOrder.Order.AmountIn)
	assert.Equal(t, "939000", txs[1].FillOrder.Order.AmountOut)
	assert.Equal(t, uint32(42161), txs[1].FillOrder.Order.SourceDomain)
}
