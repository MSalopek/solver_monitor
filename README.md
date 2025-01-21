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
    "total_gas_usd": "12.57",
    "total_gas_eth": "0.018729726074726943",
    "total_tx_count": 66,
    "network_stats": [
      {
        "total_gas_usd": "1.0755905164698336",
        "total_gas_eth": "0.000321920320268",
        "tx_count": 12,
        "network": "arbitrum"
      },
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

## Fill stats

### Endpoint `/stats/orders_filled/fill_stats`

Returns min/max fill amount and min/max revenues accross all known networks. Highest grossing orders are included in the response.

```shell
curl 'localhost:8080/stats/orders_filled/fill_stats?filler=<osmosis address>' | jq .

{
  "orders": {
    "average_revenue": "0.5998812291666666",
    "average_fill": "124.25637870833333",
    "max_fill": "500.000678", # across all networks
    "max_revenue": "20.272712", # across all networks
    "min_fill": "1", # across all networks
    "min_revenue": "0.061", # across all networks
    "max_fill_details": [ # for each network
      {
        "tx_hash": "...",
        "amount_in": "500.000678",
        "amount_out": "499.440677",
        "network": "arbitrum",
        "solver_revenue": "0.560001",
        "height": 27853947,
        "ingestion_timestamp": "2025-01-19T18:17:41.239743+01:00"
      }
    ],
    "max_revenue_details": [ # for each network
      {
        "tx_hash": "...",
        "amount_in": "500.000678",
        "amount_out": "499.440677",
        "network": "arbitrum",
        "solver_revenue": "0.560001",
        "height": 27853947,
        "ingestion_timestamp": "2025-01-19T18:17:41.239743+01:00"
      }
    ]
  }
}
```
