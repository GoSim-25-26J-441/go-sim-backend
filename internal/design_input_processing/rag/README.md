# RAG pipeline (design-input)

This package implements the **new** RAG pipeline under `/api/v1/design-input/rag`.

## Routes (Firebase auth required)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/design-input/rag` | RAG search (placeholder) |
| GET | `/api/v1/design-input/rag/search` | RAG search (placeholder) |
| POST | `/api/v1/design-input/rag` | RAG ingest/index (placeholder) |

## Structure

- `handler.go` – HTTP handlers (implement search/ingest here).
- `routes.go` – Registers routes on the `design-input` group.
- Add pipeline logic (embedding, store, retrieval) in this package or subpackages as you build.

## Legacy code

Old design-input code (jobs, ingest, graphviz, previous RAG) lives under `_legacy_files/` and is not mounted on the API. Projects chat still uses `_legacy_files/llm` (UIGP client).
