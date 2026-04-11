ALTER TABLE request_responses
    ADD COLUMN IF NOT EXISTS global_cost_recommendation JSONB;

COMMENT ON COLUMN request_responses.global_cost_recommendation IS
    'Snapshot from POST /cost/:id/recommend: best plan across regions, monthly price, rationale, computed_at';
