package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/msalopek/solver_monitor/monitor"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const API_URL = "https://osmosis-lcd.quickapi.com"
const defaultContractAddress = "osmo1vy34lpt5zlj797w7zqdta3qfq834kapx88qtgudy7jgljztj567s73ny82"

func main() {
	interval := flag.Int("interval", 1, "Polling interval in minutes")
	contractAddress := flag.String("contract-address", defaultContractAddress, "Osmosis skip-go-fast contract address to monitor.")
	logLevel := flag.String("log-level", "INFO", "Set the logging level")
	logFormat := flag.String("log-format", "json", "Set the log output format")
	configPath := flag.String("config", "config.toml", "Path to the config file")
	saveRawResponses := flag.Bool("save-raw-tx", false, "Save raw tx responses to db")
	dbPath := flag.String("db", "tx_data.db", "Path to the db file")
	loadFromFile := flag.String("load-from-file", "", "Load orders from file. If provided, all other arguments are ignored.")
	serverAddr := flag.String("server-addr", ":8080", "Server address to listen on")
	skipInitialization := flag.Bool("skip-init", false, "Skip fetching state and txs on startup. Cron job will run on interval.")
	serverOnly := flag.Bool("server-only", false, "Only run the server, don't fetch txs and dont start the cron job.")
	flag.Parse()

	// Set up logging
	if *logFormat == "json" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	} else {
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
		output.FormatLevel = func(i interface{}) string {
			return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
		}
		output.FormatMessage = func(i interface{}) string {
			return fmt.Sprintf("message: %s", i)
		}
		output.FormatFieldName = func(i interface{}) string {
			return fmt.Sprintf("%s:", i)
		}
		output.FormatFieldValue = func(i interface{}) string {
			return strings.ToUpper(fmt.Sprintf("%s", i))
		}
		log.Logger = log.Output(output)

	}

	// Set log level
	switch strings.TrimSpace(strings.ToUpper(*logLevel)) {
	case "DEBUG":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "INFO":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "WARN":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "ERROR":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	cfg := monitor.MustLoadConfig(*configPath)

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	defer db.Close()

	m := monitor.NewMonitor(db, cfg, &log.Logger, API_URL)

	// this can be done via subcommands
	if *loadFromFile != "" {
		m.LoadFromFile(*loadFromFile, *saveRawResponses)
		return
	}

	log.Logger.Debug().Strs("details", []string{
		"contract address", *contractAddress,
		"interval", strconv.Itoa(*interval)}).Msg("monitor started")

	var wg sync.WaitGroup
	if !*skipInitialization {
		// there's no do while loop in go, so we just run the orders once on startup
		log.Logger.Info().Msg("initializing state and fetching txs")
		m.RunAll(&wg, *saveRawResponses)
		wg.Wait()
		log.Logger.Info().Int("interval_minutes", *interval).Msg("initial state and txs fetched -- running cron")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ticker := time.NewTicker(time.Duration(*interval) * time.Minute)
	defer ticker.Stop()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	server := monitor.NewServer(m)

	// Start server in a goroutine with context after initial state and txs are fetched
	go func() {
		log.Info().Str("address", *serverAddr).Msg("HTTP server started")
		if err := server.RunWithContext(ctx, *serverAddr); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed to start")
		}
	}()

	for {
		select {
		case <-ticker.C:
			if !*serverOnly {
				log.Logger.Debug().Msg("interval tick -- fetching txs")
				m.RunAll(&wg, *saveRawResponses)
			}
		case <-sigs:
			log.Info().Msg("shutdown signal received")
			cancel() // Cancel the context
			log.Info().Msg("waiting for ongoing operations to complete...")
			wg.Wait() // Wait for any running goroutines to finish
			return
		case <-ctx.Done():
			log.Info().Msg("context cancelled")
			return
		}
	}
}
