package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
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

// message can be authz.MsgExec or wasmtypes.MsgExecuteContract
// the function will return the inner FillOrderEnvelope message
func (m *Monitor) getFillOrderBodyBytes(msg []byte) ([]byte, error) {
	// authzExec := authz.MsgExec{}
	wasmExec := wasmtypes.MsgExecuteContract{}

	// try as wasmExec first and try as authz if that fails
	if err := m.Codec.Unmarshal(msg, &wasmExec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal wasm: %w", err)
	}

	// this was wasmExec but it can be the wrong type (we only want fillOrder)
	if len(wasmExec.Msg.Bytes()) > 0 {
		if strings.Contains(string(wasmExec.Msg.Bytes()), "fill_order") {
			return wasmExec.Msg.Bytes(), nil
		}
		return nil, fmt.Errorf("wasmExec but not fillOrder")
	}

	// for some reason there's no error but the length on the wasm exec was 0
	// this means the message was an authz message and we need to unmarshal it
	authzExec := authz.MsgExec{}
	if err := m.Codec.Unmarshal(msg, &authzExec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal authz: %w", err)
	}

	if len(authzExec.Msgs) > 0 {
		// loop over all messages and set the first message that matches
		// fillOrder transaction as the wasmExec to be returned
		for _, msg := range authzExec.Msgs {
			temp := wasmtypes.MsgExecuteContract{}
			if strings.Contains(string(msg.Value), "fill_order") {
				if err := m.Codec.Unmarshal(msg.Value, &temp); err != nil {
					return nil, fmt.Errorf("found fillOrder but failed to unmarshal %w", err)
				}
				// fillOrder was found and processed
				wasmExec = temp
				break
			}
			continue
		}
		if wasmExec.Msg == nil || len(wasmExec.Msg.Bytes()) == 0 {
			return nil, fmt.Errorf("failed to unmarshal wasm inside authz - no fillOrder found")
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
