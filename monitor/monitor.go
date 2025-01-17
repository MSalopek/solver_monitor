package monitor

import (
	"database/sql"
	"encoding/json"
	"os"

	"cosmossdk.io/x/tx/decode"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-proto/anyutil"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pelletier/go-toml/v2"
	"github.com/rs/zerolog"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
)

type ChainEntry struct {
	Key      string `json:"key,omitempty" yaml:"key,omitempty" toml:"key,omitempty"`
	ApiUrl   string `json:"api_url,omitempty" yaml:"api_url,omitempty" toml:"api_url,omitempty"`
	UsdcAddr string `json:"usdc_address,omitempty" yaml:"usdc_address,omitempty" toml:"usdc_address,omitempty"`
	Address  string `json:"address,omitempty" yaml:"address,omitempty" toml:"address,omitempty"`
}

type Config struct {
	Arbitrum ChainEntry `json:"arbitrum,omitempty" yaml:"arbitrum,omitempty" toml:"arbitrum,omitempty"`
	Ethereum ChainEntry `json:"ethereum,omitempty" yaml:"ethereum,omitempty" toml:"ethereum,omitempty"`
	Osmosis  ChainEntry `json:"osmosis,omitempty" yaml:"osmosis,omitempty" toml:"osmosis,omitempty"`
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
			// types don't match -- skip
			continue
		}
		exec := wasmtypes.MsgExecuteContract{}
		if err := m.Codec.Unmarshal(anyMsg.Value, &exec); err != nil {
			// types don't match -- skip
			continue
		}

		fill := FillOrderEnvelope{}
		if err := json.Unmarshal(exec.Msg.Bytes(), &fill); err != nil {
			// types don't match -- skip
			continue
		}
		fillOrders = append(fillOrders, fill)
	}
	return fillOrders
}
