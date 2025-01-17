from query_osmo import API_URL
import requests
from models import OrderFilled
from urllib.parse import urlencode
from db import get_latest_height, insert_order_filled
import logging
import json
import argparse
import time


def fetch_new_orders(height):
    headers = {"accept": "application/json"}
    base_url = f"{API_URL}/cosmos/tx/v1beta1/txs"
    query = "wasm._contract_address='osmo1vy34lpt5zlj797w7zqdta3qfq834kapx88qtgudy7jgljztj567s73ny82' AND wasm.action='order_filled'"

    params = {
        "order_by": "ORDER_BY_DESC",
        "query": query,
    }

    encoded_params = urlencode(params)
    url = f"{base_url}?{encoded_params}"
    response = requests.get(f"{base_url}?{encoded_params}", headers=headers)
    data = response.json()

    tx_responses = data["tx_responses"]
    new = []
    for tx in tx_responses:
        if not (int(tx["height"]) > height):
            continue

        for msg in tx["tx"]["body"]["messages"]:
            new.append(
                OrderFilled(
                    tx_hash=tx["txhash"],
                    sender=msg["sender"],
                    amount_in=msg["msg"]["fill_order"]["order"]["amount_in"],
                    amount_out=msg["msg"]["fill_order"]["order"]["amount_out"],
                    source_domain=msg["msg"]["fill_order"]["order"]["source_domain"],
                    solver_revenue=int(msg["msg"]["fill_order"]["order"]["amount_in"])
                    - int(msg["msg"]["fill_order"]["order"]["amount_out"]),
                    height=tx["height"],
                    code=tx["code"],
                )
            )
    return new


def job(log_level=logging.INFO, log_format="json", address=None):
    if log_format == "json":
        logging.basicConfig(
            level=log_level,
            format='{"timestamp": "%(asctime)s", "level": "%(levelname)s", "message": %(message)s}',
            datefmt="%Y-%m-%d %H:%M:%S",
        )
    else:
        logging.basicConfig(
            level=log_level,
            format="%(asctime)s - %(levelname)s - %(message)s",
            datefmt="%Y-%m-%d %H:%M:%S",
        )
    latest_height = get_latest_height()
    new = fetch_new_orders(latest_height)

    if new:
        min_height = min(tx.height for tx in new)
        max_height = max(tx.height for tx in new)
        log_message = {
            "event": "transactions_collected",
            "count": len(new),
            "min_height": min_height,
            "max_height": max_height,
        }
        logging.info(json.dumps(log_message))
    else:
        log_message = {"event": "no_transactions_collected"}
        logging.info(json.dumps(log_message))

    for tx in new:
        log_message = {"event": "transaction_inserted", "tx_hash": tx.tx_hash}

        insert_order_filled(tx)
        logging.debug(json.dumps(log_message))
        if address and tx.sender == address:
            logging.info(json.dumps(tx.to_dict()))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--interval", type=int, default=1, help="Polling interval in minutes"
    )
    parser.add_argument(
        "--address",
        type=str,
        help="Address to filter transactions. When address is provided, info logs that are related to the address activity are printed.",
    )
    parser.add_argument(
        "--log-level",
        type=str,
        default="INFO",
        choices=["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"],
        help="Set the logging level",
    )
    parser.add_argument(
        "--log-format",
        type=str,
        default="json",
        choices=["text", "json"],
        help="Set the log output format",
    )
    args = parser.parse_args()
    try:
        while True:
            job(args.log_level, args.log_format, args.address)
            time.sleep(args.interval * 60)
    except KeyboardInterrupt:
        logging.info("shutting down")
