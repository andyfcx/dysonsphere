CREATE TABLE agent_credentials (
    host_id          UUID PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    token_hash       TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rotated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_credentials_token_hash ON agent_credentials(token_hash);
