# AMG/APD → scenario YAML (draft generation)

## Supported output (scenario v2)

The generator in `internal/realtime_system_simulation/amg_apd_scenario` builds **local DTOs** (`ScenarioDoc`, `ServiceDoc`, etc.) with YAML tags aligned to simulation-core scenario-v2, marshals them to YAML, and applies **non-authoritative** structural checks (`ValidateScenarioDraft`).

**Authoritative** validation and runtime preflight are always done by the simulation engine over HTTP: `POST {SIMULATION_ENGINE_URL}/v1/scenarios:validate` (see `SimulationEngineClient.ValidateScenario` and `validateScenarioPreflight` in the HTTP layer).

The backend sends JSON `{"scenario_yaml":"<string>","mode":"preflight"}`. The engine should perform **semantic** checks (not YAML syntax alone), for example that every workload `to` (or `target`) resolves to an existing service endpoint, every downstream `to` resolves, queue/topic `consumer_target` resolves where applicable, and placement/resource init succeeds. Prefer stable `code` values (e.g. `UNKNOWN_WORKLOAD_ENDPOINT`, `UNKNOWN_DOWNSTREAM_ENDPOINT`, `PLACEMENT_INFEASIBLE`) and a `path` field on each error for UI anchoring. Responses with `valid: false` may use HTTP **200**, **400**, or **422**; go-sim-backend maps structured `valid:false` bodies to **422** for the frontend. Transport failures and engine **5xx** map to **502/503**, not **422**.

Emitted fields include where applicable:

- `metadata.schema_version` (`0.2.0`)
- Per-service **`kind`**, **`role`**, **`scaling`**, **`routing`** (api_gateway only), **`behavior.queue` / `behavior.topic`**, **`placement`** (omitted unless AMG adds data later)
- Per-downstream-call **`mode`**, **`kind`**, **`probability`**, **`call_count_mean`**, **`call_latency_ms`**

AMG/APD **type**, **role**, dependency **kind**, and **sync** drive classification and downstream semantics.

## Go module

This backend **does not** import `github.com/GoSim-25-26J-441/simulation-core` as a Go module. The `go.mod` guard test `TestGoModHasNoSimulationCoreDependency` enforces that.

## AMG → scenario draft mapping

| AMG/APD input | Generator output |
|----------------|------------------|
| `type` / `role`: api_gateway, gateway, bff, web-ui (type), `role` ingress/bff | `Kind: api_gateway`, `Role: ingress`, `/ingress`, `Routing.strategy: least_queue` |
| Name hints: `web-ui`, `frontend`, `client` in id (not substring `bff` alone) with plain `service` type | `Kind: service`, `Role: ingress`, CRUD endpoints |
| database, datastore, db, postgres, mysql | `Kind: database`, `Role: datastore`, `Model: db_latency`, conservative scaling |
| `type: queue` | `Kind: queue`, `Behavior.Queue` with defaults + `consumer_target` → first app service `:/read` |
| `type: topic` | `Kind: topic`, `Behavior.Topic` with partitions/capacity/subscriber → first app service `:/read` |
| `topics[]` top-level entries | Additional `Kind: topic` services (same as inline topic services) |
| default | `Kind: service` |
| dependency `sync: true` | `DownstreamCall.Mode: sync` |
| dependency `sync: false` | `DownstreamCall.Mode: async` |
| dependency `kind` rest/grpc/queue/topic/external | `DownstreamCall.Kind` (normalized; targets to **database** services always use **`db`**) |
| target database | path `/query`, kind `db` |
| target queue / topic | path `/enqueue` / `/events`, kind `queue` / `topic` |
| default service target | path `/read` |

Implementation lives in `generate.go`, `scenario_doc.go`, and related tests.
