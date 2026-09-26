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
-- github_repo_id is nullable: a review starts before GitHub has been queried.
-- GitHub names are case-insensitive, so uniqueness ignores case.
CREATE TABLE repositories (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    owner          TEXT NOT NULL,
    name           TEXT NOT NULL,
    github_repo_id BIGINT UNIQUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX repositories_name_idx ON repositories (lower(owner), lower(name));

-- Title, author and stats are empty until the review has fetched the PR.
CREATE TABLE pull_requests (
    id            BIGSERIAL PRIMARY KEY,
    repository_id BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    number        INTEGER NOT NULL CHECK (number >= 1),
    title         TEXT NOT NULL DEFAULT '',
    author        TEXT NOT NULL DEFAULT '',
    url           TEXT NOT NULL DEFAULT '',
    state         TEXT NOT NULL DEFAULT '',
    head_sha      TEXT NOT NULL DEFAULT '',
    files_changed INTEGER NOT NULL DEFAULT 0,
    additions     INTEGER NOT NULL DEFAULT 0,
    deletions     INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
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
-- status_message/status_at hold the latest progress line so a client that
-- connects mid-review can be told where things stand.
CREATE TABLE reviews (
    id              BIGSERIAL PRIMARY KEY,
    pull_request_id BIGINT NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    head_sha        TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'queued'
                    CHECK (status IN ('queued', 'fetching_pr', 'analyzing', 'reviewing', 'completed', 'failed')),
    status_message  TEXT NOT NULL DEFAULT '',
    status_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    error_code      TEXT,
    error_message   TEXT,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((status = 'failed') = (error_code IS NOT NULL)),
    CHECK ((status IN ('completed', 'failed')) = (completed_at IS NOT NULL))
);
CREATE INDEX reviews_created_idx ON reviews (created_at DESC, id DESC);
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
    line         INTEGER CHECK (line IS NULL OR line >= 1),         -- NULL = whole-file finding
    end_line     INTEGER CHECK (end_line IS NULL OR end_line >= line),
    evidence     TEXT NOT NULL,          -- code excerpt or measurement
    found        TEXT NOT NULL,          -- "What we found"
    why          TEXT NOT NULL,          -- "Why this matters"
    improvement  TEXT NOT NULL,          -- "Possible improvement"
    lesson_id    TEXT,                   -- lesson file id, e.g. 'error-handling'
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX findings_review_idx ON findings (review_id, id);
CREATE INDEX findings_skill_severity_idx ON findings (skill_id, severity);
