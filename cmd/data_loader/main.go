package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/msalopek/solver_monitor/monitor"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const API_URL = "https://osmosis-lcd.quickapi.com"
const defaultContractAddress = "osmo1vy34lpt5zlj797w7zqdta3qfq834kapx88qtgudy7jgljztj567s73ny82"

var (
	logLevel         string
	logFormat        string
	configPath       string
	dbPath           string
	saveRawResponses bool
	filePath         string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "data_loader",
		Short: "A tool for loading and managing orders",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			setupLogging()
		},
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "INFO", "Set the logging level (DEBUG, INFO, WARN, ERROR)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "json", "Set the log output format (json or text)")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "config.toml", "Path to the config file")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "tx_data.db", "Path to the db file")
	rootCmd.PersistentFlags().BoolVar(&saveRawResponses, "save-raw-tx", false, "Save raw tx responses to db")

	// Load command
	loadCmd := &cobra.Command{
		Use:   "load",
		Short: "Load orders from a file",
		Run: func(cmd *cobra.Command, args []string) {
			db, m := setupMonitor()
			defer db.Close()
			m.LoadFromFile(filePath, saveRawResponses)
		},
	}
	loadCmd.Flags().StringVar(&filePath, "file", "", "Load orders from file")
	loadCmd.MarkFlagRequired("file")

	// Save missing command
	saveMissingCmd := &cobra.Command{
		Use:   "save_missing",
		Short: "Persist missing orders from file to db",
		Run: func(cmd *cobra.Command, args []string) {
			db, m := setupMonitor()
			defer db.Close()
			m.LoadMissingOrderFromFile(filePath)
		},
	}
	saveMissingCmd.Flags().StringVar(&filePath, "file", "", "Persist missing orders from file")
	saveMissingCmd.MarkFlagRequired("file")

	// Get orders command
	getOrdersCmd := &cobra.Command{
		Use:   "get_orders",
		Short: "Save orders to file",
		Run: func(cmd *cobra.Command, args []string) {
			db, m := setupMonitor()
			defer db.Close()
			m.GetAllOsmosisOrders(defaultContractAddress, API_URL, filePath)
		},
	}
	getOrdersCmd.Flags().StringVar(&filePath, "file", "", "Save orders to file")
	getOrdersCmd.MarkFlagRequired("file")

	rootCmd.AddCommand(loadCmd, saveMissingCmd, getOrdersCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func setupLogging() {
	// Set up logging
	if logFormat == "json" {
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
	switch strings.TrimSpace(strings.ToUpper(logLevel)) {
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
}

func setupMonitor() (*sql.DB, *monitor.Monitor) {
	cfg := monitor.MustLoadConfig(configPath)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	return db, monitor.NewMonitor(db, cfg, &log.Logger, API_URL)
}
