from datetime import datetime
from dataclasses import dataclass, field


@dataclass
class OrderFilled:
    tx_hash: str
    sender: str
    amount_in: str
    amount_out: str
    source_domain: str
    solver_revenue: int
    height: int
    code: int
    ingestion_timestamp: datetime = field(default_factory=datetime.now)

    def to_dict(self) -> dict:
        return {
            "tx_hash": self.tx_hash,
            "sender": self.sender,
            "amount_in": self.amount_in,
            "amount_out": self.amount_out,
            "source_domain": self.source_domain,
            "solver_revenue": self.solver_revenue,
            "height": self.height,
            "code": self.code,
            "ingestion_timestamp": self.ingestion_timestamp.isoformat()
        }

    @classmethod
    def from_dict(cls, data: dict) -> "OrderFilled":
        if "ingestion_timestamp" in data:
            data["ingestion_timestamp"] = datetime.fromisoformat(data["ingestion_timestamp"])
        return cls(
            tx_hash=data["tx_hash"],
            sender=data["sender"],
            amount_in=data["amount_in"],
            amount_out=data["amount_out"],
            source_domain=data["source_domain"],
            solver_revenue=data["solver_revenue"],
            height=data["height"],
            code=data["code"],
        )
