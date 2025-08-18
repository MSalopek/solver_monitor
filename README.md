# Usage of ./solver_monitor

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

### Endpoint `/stats/orders_filled/fills_in_range`

Returns aggregated data about orders executed in given range, alongside volume and the fill amounts (min, avg, max).

**Required args**

- `network`: arbitrum, base, ethereum

**Optional args**

- `filler` - solver osmosis address to filter by
- `start_block` - reduces the output set; it will start from `start_block` - earlier blocks are ignored

Example:

```shell
curl 'localhost:8080/stats/orders_filled/fills_in_range?network=ethereum&filler=osmo1xjuvq8mlmhc24l2ewya2uyyj9t6r0dcfdhza6h&start_block=27354010' | jq .
{
  "orders": [
    {
      "amount_range": "250-500",
      "total_orders_in_range": 7,
      "executed_orders_in_range": 3,
      "filler": "osmo1xjuvq8mlmhc24l2ewya2uyyj9t6r0dcfdhza6h",
      "total_volume": 1061,
      "avg_transfer": 353.759515,
      "min_transfer": 262,
      "max_transfer": 450,
      "source_domain": 1
    }
  ]
}
```

2025-08-14T10:36:16-04:00 ERR failed to get BASE txs error="json: cannot unmarshal string into Go struct field EthScanTxListResponse.result of type []monitor.EthTxDetails"
2025-08-14T10:36:16-04:00 ERR failed to get arbitrum txs error="json: cannot unmarshal string into Go struct field EthScanTxListResponse.result of type []monitor.EthTxDetails"

3ZQUQBDTNJSX8K8KX1SA99XQTKECT6M152

```json
         "blockNumber":"14923678",
         "blockHash":"0x7e1638fd2c6bdd05ffd83c1cf06c63e2f67d0f802084bef076d06bdcf86d1bb0",
         "timeStamp":"1654646411",
         "hash":"0xc52783ad354aecc04c670047754f062e3d6d04e8f5b24774472651f9c3882c60",
         "nonce":"1",
         "transactionIndex":"61",
         "from":"0x9aa99c23f67c81701c772b106b4f83f6e858dd2e",
         "to":"",
         "value":"0",
         "gas":"6000000",
         "gasPrice":"83924748773",
         "input":"",
         "methodId":"0x61016060",
         "functionName":""
         "contractAddress":"0xc5102fe9359fd9a28f877a67e36b0f050d81a3cc",
         "txreceipt_status":"1",
         "gasUsed":"4457269",
         "confirmations":"122485",
         "isError":"0",
         "cumulativeGasUsed":"10450178",

```
