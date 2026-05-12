CREATE TABLE enrollment_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash TEXT        NOT NULL UNIQUE,
    label      TEXT        NOT NULL DEFAULT '',
    used       BOOLEAN     NOT NULL DEFAULT FALSE,
    used_at    TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
