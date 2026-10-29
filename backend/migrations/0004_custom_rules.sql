-- A user's own review standards, written in plain English.
CREATE TABLE custom_rules (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title        TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
    instructions TEXT NOT NULL CHECK (char_length(instructions) BETWEEN 1 AND 1000),
    severity     TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'suggestion')),
    enabled      BOOLEAN NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX custom_rules_user_idx ON custom_rules (user_id, id);

-- The rules a review used, copied when it starts, so later edits never change old reviews.
CREATE TABLE review_rules (
    review_id    BIGINT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    rule_id      BIGINT NOT NULL,
    title        TEXT NOT NULL,
    instructions TEXT NOT NULL,
    severity     TEXT NOT NULL,
    PRIMARY KEY (review_id, rule_id)
);
