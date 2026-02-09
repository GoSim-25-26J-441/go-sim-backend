CPU sizing guide
Rule-of-thumb (Go services, light IO):
- 200 RPS @ 150ms p95: 2–4 vCPU (baseline), autoscale to 6–8.
- gRPC is ~10–20% more efficient than REST for small payloads.
- If TLS termination on app, add ~0.5–1 vCPU per 200 RPS.
- For DB-bound endpoints, scale read replicas/caches first.
