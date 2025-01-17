import requests
import time
from models import OrderFilled
from urllib.parse import urlencode
import os
import json
from db import init_db, insert_order_filled

API_URL = "https://osmosis-lcd.quickapi.com"
# API_URL = "https://lcd.osmosis.zone"


def get_all_orders():
    all_txs = []
    all_tx_responses = []
    headers = {"accept": "application/json"}
    base_url = f"{API_URL}/cosmos/tx/v1beta1/txs"
    query = "wasm._contract_address='osmo1vy34lpt5zlj797w7zqdta3qfq834kapx88qtgudy7jgljztj567s73ny82' AND wasm.action='order_filled'"

    attempts = 0
    max_attempts = 25
    timestamp = int(time.time())
    total = 0
    while attempts < max_attempts:
        params = {
            "limit": "100",
            "page": attempts + 1,
            "query": query,
        }

        encoded_params = urlencode(params)
        url = f"{base_url}?{encoded_params}"
        # print(url)
        response = requests.get(f"{base_url}?{encoded_params}", headers=headers)
        data = response.json()

        if data.get("total"):
            total = int(data["total"])
            print(total, "HAVE", len(all_txs))
            if len(all_txs) >= int(total):
                print("COLLECTED ALL")
                break

        if not data.get("txs"):
            break

        all_txs.extend(data["txs"])
        all_tx_responses.extend(data["tx_responses"])
        print("GOT", len(data["tx_responses"]))

        filename = f"./orders/orders_{timestamp}_{attempts}.json"
        attempts += 1
        time.sleep(1)

    with open(filename, "w") as f:
        json.dump({"txs": all_txs, "tx_responses": all_tx_responses}, f)


def handle_files():
    txs_details = []
    files = [f for f in os.listdir("./orders") if f.endswith(".json")]
    for file in files:
        # print("FILE", file)
        data = {}
        if file.startswith("orders_"):
            fname = f"./orders/{file}"
            with open(fname, "r") as f:
                data = json.load(f)

        for tx in data["tx_responses"]:
            for msg in tx["tx"]["body"]["messages"]:
                txs_details.append(
                    OrderFilled(
                        tx_hash=tx["txhash"],
                        sender=msg["sender"],
                        amount_in=msg["msg"]["fill_order"]["order"]["amount_in"],
                        amount_out=msg["msg"]["fill_order"]["order"]["amount_out"],
                        source_domain=msg["msg"]["fill_order"]["order"][
                            "source_domain"
                        ],
                        solver_revenue=int(
                            msg["msg"]["fill_order"]["order"]["amount_in"]
                        )
                        - int(msg["msg"]["fill_order"]["order"]["amount_out"]),
                        height=tx["height"],
                        code=tx["code"],
                    )
                )
    with open("txs_details.json", "w") as f:
        json.dump([order.to_dict() for order in txs_details], f)
    return txs_details


if __name__ == "__main__":
    get_all_orders()
    # init_db()
    txs_details = handle_files()
    print(txs_details)
    dedupe = list(set([tx.tx_hash for tx in txs_details]))
    print(len(dedupe), len(txs_details))

    # for tx in txs_details:
    #     insert_order_filled(tx)
