import json
from db import insert_raw_tx_response, init_db
from models import OrderFilled
import os


def insert_raw():
    files = [f for f in os.listdir("./orders") if f.endswith(".json")]
    for file in files:
        data = {}
        with open(f"./orders/{file}", "r") as f:
            data = json.load(f)
        for tx in data["tx_responses"]:
            insert_raw_tx_response(tx)


if __name__ == "__main__":
    init_db()
    insert_raw()
