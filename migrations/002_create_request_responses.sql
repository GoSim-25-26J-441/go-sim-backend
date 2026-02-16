CREATE TABLE IF NOT EXISTS request_responses (
    id             uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id        VARCHAR(128) NOT NULL,
    request        JSONB        NOT NULL,
    response       JSONB        NOT NULL,
    best_candidate JSONB        NOT NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

ALTER TABLE request_responses
    ADD CONSTRAINT fk_request_responses_user
    FOREIGN KEY (user_id)
    REFERENCES users(firebase_uid)
    ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_request_responses_user_id
    ON request_responses(user_id);

CREATE INDEX IF NOT EXISTS idx_request_responses_created_at
    ON request_responses(created_at);