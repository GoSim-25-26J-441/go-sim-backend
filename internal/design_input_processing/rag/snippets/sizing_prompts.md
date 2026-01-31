Sizing prompts
Ask the user:
- Peak vs average RPS? Daily/weekly seasonality?
- p95 latency target? Allowable queueing?
- Read/write split? Payload size (KB)?
- Cache hit rate expected? DB QPS budget?
- Critical flows? (login, checkout, payment)
- Availability target (SLA/SLO)? Multi-AZ/region?
- Current infra (CPU model, core count, RAM, network)?
- Burst tolerance (x times normal) and warmup?
