-- Core schema for the PR reviewer.
-- Relationships: user -> repository -> pull_request -> review -> finding -> skill.
-- Text + CHECK is used instead of enums so values can change with a plain migration.

CREATE TABLE users (
    id         BIGSERIAL PRIMARY KEY,
    github_id  BIGINT UNIQUE NOT NULL,
    name       TEXT NOT NULL,
    email      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- user_id is nullable: V1 reviews public PRs by URL with no login.
CREATE TABLE repositories (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    github_repo_id BIGINT UNIQUE NOT NULL,
    owner          TEXT NOT NULL,
    name           TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner, name)
);

CREATE TABLE pull_requests (
    id            BIGSERIAL PRIMARY KEY,
    repository_id BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    number        INTEGER NOT NULL,
    title         TEXT NOT NULL,
    author        TEXT NOT NULL,
    head_sha      TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repository_id, number)
);

-- Mirror of the skills/ directory, synced at startup. The YAML files stay the
-- source of truth; this table exists so findings can reference a skill by FK.
CREATE TABLE skills (
    id        TEXT PRIMARY KEY,          -- slug, e.g. 'go'
    name      TEXT NOT NULL,
    version   INTEGER NOT NULL,
    languages TEXT[] NOT NULL
);

-- One review = one analysis run of a PR at a specific head commit.
CREATE TABLE reviews (
    id              BIGSERIAL PRIMARY KEY,
    pull_request_id BIGINT NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    head_sha        TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    error           TEXT,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX reviews_pull_request_idx ON reviews (pull_request_id, created_at DESC);

-- A finding is a teaching unit: evidence, why it matters, how to improve.
CREATE TABLE findings (
    id           BIGSERIAL PRIMARY KEY,
    review_id    BIGINT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    skill_id     TEXT NOT NULL REFERENCES skills(id),
    check_id     TEXT NOT NULL,          -- check within the skill, e.g. 'ignored-error'
    source       TEXT NOT NULL CHECK (source IN ('deterministic', 'ai')),
    severity     TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'suggestion')),
    confidence   REAL NOT NULL CHECK (confidence BETWEEN 0 AND 1),  -- 1.0 for deterministic
    file         TEXT NOT NULL,
    line         INTEGER NOT NULL,
    end_line     INTEGER,
    evidence     TEXT NOT NULL,          -- code excerpt or measurement ("cyclomatic complexity 18")
    found        TEXT NOT NULL,          -- "What we found"
    why          TEXT NOT NULL,          -- "Why this matters"
    improvement  TEXT NOT NULL,          -- "Possible improvement"
    lesson       TEXT,                   -- lesson file id, e.g. 'error-handling'
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX findings_review_idx ON findings (review_id);
CREATE INDEX findings_skill_severity_idx ON findings (skill_id, severity);
