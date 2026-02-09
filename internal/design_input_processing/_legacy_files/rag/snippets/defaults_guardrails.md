Defaults & guardrails
If unknown:
- p95 latency: 150ms; p99: 400ms
- Burst: 2x average for 5 minutes
- Cache hit rate: 60% (keyed reads)
- Error budget: 0.1% per month
- Health checks every 10s, 3 fails to mark unhealthy
Use these until the user provides better numbers.
