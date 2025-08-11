package monitor

import (
	"database/sql"
	"encoding/json"
	"os"
	"sync"
	"time"

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
	ethRetry := 0
	arbitrumRetry := 0
	baseRetry := 0
	maxRetry := 5
	limitSleep := 60 * time.Second

	wg.Add(6)
	go func() {
		defer wg.Done()
		m.RunOrders(saveRawResponses)
	}()
	go func() {
		defer wg.Done()
		m.RunOsmosisBalances()
	}()
	go func() {
		defer wg.Done()
		m.RunAvalancheBalances()
		time.Sleep(limitSleep)
		m.RunAvalancheTxHistory(saveRawResponses)
	}()
	go func() {
		defer wg.Done()
		for {
			code := m.RunEthereumBalances()
			if code != 200 {
				logMsg := HttpCodeCheck(code)
				m.logger.Error().Msgf("%s, code: %d", logMsg, code)
				time.Sleep(limitSleep)
				ethRetry++
				if ethRetry > maxRetry {
					m.logger.Error().Msg("Ethereum balances RPC query retries exceeded, exiting")
					os.Exit(1)
				}
				continue
			}
			break
		}
		m.RunEthereumTxHistory(saveRawResponses)
	}()
	go func() {
		defer wg.Done()
		for {
			code := m.RunArbitrumBalances()
			if code != 200 {
				logMsg := HttpCodeCheck(code)
				m.logger.Error().Msgf("%s, code: %d", logMsg, code)
				time.Sleep(limitSleep)
				arbitrumRetry++
				if arbitrumRetry > maxRetry {
					m.logger.Error().Msg("Arbitrum balances RPC query retries exceeded, exiting")
					os.Exit(1)
				}
				continue
			}
			break
		}
		m.RunArbitrumTxHistory(saveRawResponses)
	}()
	go func() {
		defer wg.Done()
		for {
			code := m.RunBaseBalances()
			if code != 200 {
				logMsg := HttpCodeCheck(code)
				m.logger.Error().Msgf("%s, code: %d", logMsg, code)
				time.Sleep(limitSleep)
				baseRetry++
				if baseRetry > maxRetry {
					m.logger.Error().Msg("Base balances RPC query retries exceeded, exiting")
					os.Exit(1)
				}
				continue
			}
			break
		}
		m.RunBaseTxHistory(saveRawResponses)
	}()
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
