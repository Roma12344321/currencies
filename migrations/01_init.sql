CREATE TABLE IF NOT EXISTS quote_requests
(
    id         UUID PRIMARY KEY,
    pair       VARCHAR(7)    NOT NULL,
    status     VARCHAR(20)   NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_quote_requests_pending ON quote_requests (created_at) WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS quotes
(
    pair       VARCHAR(7)  NOT NULL,
    price      NUMERIC     NOT NULL,
    request_id UUID        NOT NULL REFERENCES quote_requests (id) ON DELETE CASCADE,
    updated_at TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (pair)
);
