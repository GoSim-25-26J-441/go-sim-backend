-- AMG-APD versioning is now stored in the unified diagram_versions table
-- defined in 0002_projects_chats_diagrams.sql. This migration is retained
-- only for backwards compatibility; the dedicated amg_apd_versions table is
-- removed if it exists.

DROP TABLE IF EXISTS amg_apd_versions;

-- AMG-APD versioning: store analyses per user_id + chat_id for history, compare, delete
-- Uses PostgreSQL; connection details from .env (DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME)

CREATE TABLE IF NOT EXISTS amg_apd_versions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         VARCHAR(255) NOT NULL,
    chat_id         VARCHAR(255) NOT NULL,
    version_number  INT NOT NULL,
    title           VARCHAR(512) NOT NULL DEFAULT 'Untitled',
    yaml_content    TEXT NOT NULL,
    graph_json      JSONB NOT NULL,
    dot_content     TEXT,
    detections_json JSONB NOT NULL DEFAULT '[]',
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE (user_id, chat_id, version_number)
);

CREATE INDEX IF NOT EXISTS idx_amg_apd_versions_user_chat
    ON amg_apd_versions (user_id, chat_id);

CREATE INDEX IF NOT EXISTS idx_amg_apd_versions_created
    ON amg_apd_versions (user_id, chat_id, created_at DESC);

COMMENT ON TABLE amg_apd_versions IS 'AMG-APD analysis versions per user/chat for versioning and side-by-side compare';
