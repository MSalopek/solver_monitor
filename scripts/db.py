import sqlite3
import json
from models import OrderFilled

conn = sqlite3.connect("tx_data.db")


def init_db():
    conn.execute(
        """
        CREATE TABLE IF NOT EXISTS tx_data (
            tx_hash TEXT PRIMARY KEY,
            sender TEXT,
            amount_in INTEGER,
            amount_out INTEGER,
            source_domain TEXT,
            solver_revenue INTEGER,
            code TEXT,
            height INTEGER,
            ingestion_timestamp DATETIME DEFAULT CURRENT_TIMESTAMP

        )
    """
    )
    conn.execute(
        """
        CREATE TABLE IF NOT EXISTS raw_tx_responses (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            tx_hash TEXT,
            height INTEGER,
            tx_response JSON,
            valid BOOLEAN
        )
    """
    )
    conn.commit()


def insert_raw_tx_response(tx_response: dict):
    cursor = conn.cursor()
    cursor.execute(
        """
        INSERT INTO raw_tx_responses (tx_hash, height, tx_response, valid)
        VALUES (?, ?, ?, ?)
    """,
        (
            tx_response["txhash"],
            tx_response["height"],
            json.dumps(tx_response),
            tx_response["code"] == 0,
        ),
    )
    conn.commit()


def insert_order_filled(order: OrderFilled):
    cursor = conn.cursor()
    cursor.execute(
        """
        INSERT INTO tx_data (tx_hash, sender, amount_in, amount_out, source_domain, solver_revenue, height, code, ingestion_timestamp)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    """,
        (
            order.tx_hash,
            order.sender,
            order.amount_in,
            order.amount_out,
            order.source_domain,
            order.solver_revenue,
            order.height,
            order.code,
            order.ingestion_timestamp,
        ),
    )
    conn.commit()


def read_orders_filled():
    cursor = conn.cursor()
    cursor.execute(
        "SELECT tx_hash, sender, amount_in, amount_out, source_domain, solver_revenue, height, code, ingestion_timestamp FROM tx_data"
    )
    rows = cursor.fetchall()
    return [OrderFilled(*row) for row in rows]


def read_orders_by_sender(conn, sender: str):
    cursor = conn.cursor()
    cursor.execute(
        """
        SELECT tx_hash, sender, amount_in, amount_out, source_domain, solver_revenue, height, code, ingestion_timestamp
        FROM tx_data
        WHERE sender = ?
    """,
        (sender,),
    )
    rows = cursor.fetchall()
    return [OrderFilled(*row) for row in rows]


def get_latest_height():
    cursor = conn.cursor()
    cursor.execute("SELECT MAX(height) FROM tx_data")
    result = cursor.fetchone()
    return result[0] if result[0] is not None else 0


if __name__ == "__main__":
    init_db()
