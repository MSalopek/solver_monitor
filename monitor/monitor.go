package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"cosmossdk.io/x/tx/decode"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-proto/anyutil"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pelletier/go-toml/v2"
	"github.com/rs/zerolog"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
)

type ChainEntry struct {
	Key         string `json:"key,omitempty" yaml:"key,omitempty" toml:"key,omitempty"`
	ApiUrl      string `json:"api_url,omitempty" yaml:"api_url,omitempty" toml:"api_url,omitempty"`
	UsdcAddress string `json:"usdc_address,omitempty" yaml:"usdc_address,omitempty" toml:"usdc_address,omitempty"`
	Address     string `json:"address,omitempty" yaml:"address,omitempty" toml:"address,omitempty"`
}

type SolverConfig struct {
	SolverAddress   string `json:"solver_address,omitempty" yaml:"solver_address,omitempty" toml:"solver_address,omitempty"`
	ContractAddress string `json:"contract_address,omitempty" yaml:"contract_address,omitempty" toml:"contract_address,omitempty"`
}

type OsmosisConfig struct {
	ChainEntry
	SolverConfig
}

type Config struct {
	Arbitrum  ChainEntry    `json:"arbitrum,omitempty" yaml:"arbitrum,omitempty" toml:"arbitrum,omitempty"`
	Ethereum  ChainEntry    `json:"ethereum,omitempty" yaml:"ethereum,omitempty" toml:"ethereum,omitempty"`
	Base      ChainEntry    `json:"base,omitempty" yaml:"base,omitempty" toml:"base,omitempty"`
	Osmosis   OsmosisConfig `json:"osmosis,omitempty" yaml:"osmosis,omitempty" toml:"osmosis,omitempty"`
	Avalanche ChainEntry    `json:"avalanche,omitempty" yaml:"avalanche,omitempty" toml:"avalanche,omitempty"`
}

func MustLoadConfig(path string) *Config {
	cfg := &Config{}
	// open the file path
	file, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	if err = toml.Unmarshal(file, cfg); err != nil {
		panic(err)
	}
	return cfg
}

type Monitor struct {
	Codec             codec.Codec
	interfaceRegistry types.InterfaceRegistry
	amino             *codec.LegacyAmino
	decoder           *decode.Decoder
	db                *sql.DB
	cfg               *Config
	logger            *zerolog.Logger
	apiUrl            string
}

func NewMonitor(db *sql.DB, cfg *Config, logger *zerolog.Logger, apiUrl string) *Monitor {
	InitDB(db)

	enc := MakeEncodingConfig()
	return &Monitor{
		Codec:             enc.Marshaler,
		interfaceRegistry: enc.InterfaceRegistry,
		amino:             enc.Amino,
		decoder:           MustInitDecoder(),
		db:                db,
		cfg:               cfg,
		logger:            logger,
		apiUrl:            apiUrl,
	}
}

func (m *Monitor) RunAll(wg *sync.WaitGroup, saveRawResponses bool) {
	wg.Add(10)
	go func() {
		defer wg.Done()
		m.RunOsmosisBalances()
	}()
	go func() {
		defer wg.Done()
		m.RunAvalancheBalances()
	}()
	go func() {
		defer wg.Done()
		m.RunEthereumBalances()
	}()
	go func() {
		defer wg.Done()
		m.RunArbitrumBalances()
	}()
	go func() {
		defer wg.Done()
		m.RunBaseBalances()
	}()
	go func() {
		defer wg.Done()
		m.RunOrders(saveRawResponses)
	}()
	go func() {
		defer wg.Done()
		m.RunArbitrumTxHistory(saveRawResponses)
	}()
	go func() {
		defer wg.Done()
		m.RunEthereumTxHistory(saveRawResponses)
	}()
	go func() {
		defer wg.Done()
		m.RunBaseTxHistory(saveRawResponses)
	}()
	go func() {
		defer wg.Done()
		m.RunAvalancheTxHistory(saveRawResponses)
	}()
}

// message can be authz.MsgExec or wasmtypes.MsgExecuteContract
// the function will return the inner FillOrderEnvelope message
func (m *Monitor) getFillOrderBodyBytes(msg []byte) ([]byte, error) {
	// authzExec := authz.MsgExec{}
	wasmExec := wasmtypes.MsgExecuteContract{}

	// try as wasmExec first and try as authz if that fails
	if err := m.Codec.Unmarshal(msg, &wasmExec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal wasm: %w", err)
	}

	// the message was not an authz message and this does the job
	if len(wasmExec.Msg.Bytes()) > 0 {
		return wasmExec.Msg.Bytes(), nil
	}

	// for some reason there's no error but the length on the wasm exec was 0
	// this means the message was an authz message and we need to unmarshal it
	authzExec := authz.MsgExec{}
	if err := m.Codec.Unmarshal(msg, &authzExec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal authz: %w", err)
	}

	if len(authzExec.Msgs[0].Value) > 0 {
		if err := m.Codec.Unmarshal(authzExec.Msgs[0].Value, &wasmExec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal wasm inside authz: %w", err)
		}
	}

	return wasmExec.Msg.Bytes(), nil
}

func (m *Monitor) DecodeTxResponse(r *sdktypes.TxResponse) []FillOrderEnvelope {
	fillOrders := []FillOrderEnvelope{}
	decodedTx, err := m.decoder.Decode(r.Tx.Value)
	if err != nil {
		return fillOrders
	}

	// fmt.Println(decodedTx)
	for _, msg := range decodedTx.Messages {
		anyMsg, err := anyutil.New(msg)
		if err != nil {
			continue
		}

		fillOrderBody, err := m.getFillOrderBodyBytes(anyMsg.Value)
		if err != nil {
			continue
		}

		fill := FillOrderEnvelope{}
		if err := json.Unmarshal(fillOrderBody, &fill); err != nil {
			// types don't match -- skip
			continue
		}
		fillOrders = append(fillOrders, fill)
	}
	return fillOrders
}
