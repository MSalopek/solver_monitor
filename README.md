# Usage of ./solver_monitor:

```
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
  -server-addr string
    	Server address to listen on (default ":8080")
  -server-only
    	Only run the server, don't fetch txs and dont start the cron job.
  -skip-init
    	Skip fetching state and txs on startup. Cron job will run on interval.
```

# API interface

## Aggregated fees

### ENDPOINT: `/stats/fees`

Returns fee amounts (ethereum) paid across all networks that the solver has indexed.

**Params**

- `as_integer` - causes all values to be returned as strings representing integer values; otherwise returns strings representing decimals
  - usage `localhost:8080/stats/fees?as_integer=true`

#### Example

```shell
curl 'localhost:8080/stats/fees' | jq .
{
  "fees": {
    "total_gas_used": "0.000973216190484",
    "total_tx_count": 50,
    "network_stats": [
      {
        "total_gas_used": "0.000295746839457",
        "tx_count": 46,
        "network": "arbitrum"
      },
      {
        "total_gas_used": "0.000677469351027",
        "tx_count": 4,
        "network": "ethereum"
      }
    ]
  }
}
```

## Filled orders

### Endpoint `/stats/orders_filled`

Returns solver fill order statistics: total orders, total revenue and revenue per-network.

**Params**

- `as_integer` - causes all values to be returned as strings representing integer values; otherwise returns strings representing decimals
  - usage `localhost:8080/stats/orders_filled?filler=<osmosis-address>&as_integer=true`
- `filler` [required] - get orders for known filler address

#### Example

```shell
curl 'localhost:8080/stats/orders_filled?filler=<osmosis-address>' | jq .
{
  "orders_filled": {
    "total_solver_revenue": "25.059113", # USDC
    "total_order_count": "34",
    "networks": [
      {
        "total_solver_revenue": "20.272712", # USDC
        "order_count": "1",
        "network": "ethereum"
      },
      {
        "total_solver_revenue": "4.786401", # USDC
        "order_count": "33",
        "network": "arbitrum"
      }
    ]
  }
}
```

## Balances (latest)

### Endpoint `/balances/latest`

Returns latest balancess accross all known networks (USDC, ETH, OSMO).

**Params**

- `as_integer` - causes all values to be returned as strings representing integer values; otherwise returns strings representing decimals
  - usage `localhost:8080/balances/latest?as_integer=true`
  - when used the `exponent` field will get added to the respones object to help with decimal conversions

```shell
curl localhost:8080/balances/latest | jq .
{
  "balances": {
    "arbitrum": [
      {
        "timestamp": 1737301329,
        "balance": "0.041421979450543",
        "token": "ETH"
      },
      {
        "timestamp": 1737301329,
        "balance": "1507.189797",
        "token": "USDC"
      }
    ],
    "ethereum": [
      ...
    ],
    "osmosis": [
      ...
    ]
  }
}
```
