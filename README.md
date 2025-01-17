Usage of ./solver_monitor:
  -config string
    	Path to the config file (default "config.toml")
  -contract-address string
    	Osmosis skip-go-fast contract address to monitor.
  -db string
    	Path to the db file (default "tx_data.db")
  -interval int
    	Polling interval in minutes (default 1)
  -load-from-file string
    	Load orders from file. If provided, all other arguments are ignored.
  -log-format string
    	Set the log output format (default "json")
  -log-level string
    	Set the logging level (default "INFO")
  -save-raw-tx
    	Save raw tx responses to db
  -solver-address string
    	Solver address to monitor. This will be used to filter transactions.
