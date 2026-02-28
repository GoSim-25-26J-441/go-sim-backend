Throughput formula
Concurrency ≈ RPS * latency_seconds
Example: 200 RPS with p95 0.15s ⇒ ~30 in-flight.
CPU sizing heuristic:
- Start with 1 core per 50–150 RPS for typical Go REST (no heavy crypto/IO).
- Add 25–50% headroom for bursts and deployment drains.
