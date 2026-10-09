-- The address people open Polyfin at, such as https://media.example.org,
-- which links in notifications start with; empty, messages carry no link.
-- An http or https URL without a trailing slash, query or fragment.
ALTER TABLE settings
    ADD COLUMN public_address text NOT NULL DEFAULT ''
        CHECK (public_address = '' OR ((public_address LIKE 'http://%' OR public_address LIKE 'https://%') AND octet_length(public_address) <= 512));

-- Where notifications go: the server's targets (user_id NULL), which
-- administrators add and which receive the events of every user, and each
-- user's own. A webhook or Discord target keeps its address in secret,
-- sealed with POLYFIN_SECRET_KEY, and in address only its scheme and host,
-- shown in its place; an ntfy target keeps its server in address, its topic,
-- and its access token, if any, in secret. events are the events it
-- receives. problem tells a target that refused Polyfin ('refused': 401, 403,
-- 404 or 410), rejected a message ('rejected': another 4xx), or could not be
-- reached ('unreachable'), with the status it answered; a delivered message
-- clears it.
CREATE TABLE notification_targets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid REFERENCES users ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('webhook', 'discord', 'ntfy')),
    name text NOT NULL CHECK (name <> '' AND char_length(name) <= 64),
    address text NOT NULL CHECK (address LIKE 'http://%' OR address LIKE 'https://%'),
    topic text NOT NULL DEFAULT '',
    secret text NOT NULL DEFAULT '',
    events text[] NOT NULL DEFAULT '{}',
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_sent_at timestamptz,
    problem text CHECK (problem IN ('refused', 'rejected', 'unreachable')),
    problem_status integer,
    problem_at timestamptz,
    CHECK ((kind = 'ntfy') = (topic <> '')),
    CHECK (kind = 'ntfy' OR secret <> '')
);

CREATE INDEX notification_targets_user ON notification_targets (user_id, created_at);

-- The series each user follows (played or marked favorite) as the
-- new-episode check last saw them: a series first seen has the episodes it
-- already has recorded in notification_episodes without a message, so that
-- following a series, or starting to use notifications, never sends its old
-- episodes.
CREATE TABLE notification_series (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    series_id uuid NOT NULL,
    seen_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, series_id)
);

-- The episodes each user was told about, or that were available when their
-- series was first seen: one message per episode and user. The server's
-- targets are told about an episode once, whoever follows it.
CREATE TABLE notification_episodes (
    user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
    episode_id uuid NOT NULL,
    seen_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, episode_id)
);

CREATE TABLE notification_server_episodes (
    episode_id uuid PRIMARY KEY,
    seen_at timestamptz NOT NULL
);

-- The problems System › Health shows, as the health check last found them,
-- with the page of the admin app that shows each: seen counts the checks
-- in a row that found a problem, missing those that no longer did;
-- announced tells that its message was sent, so that a restart neither
-- sends it again nor forgets to tell when it is solved.
CREATE TABLE notification_health (
    key text PRIMARY KEY,
    severity text NOT NULL CHECK (severity IN ('error', 'warning')),
    text text NOT NULL,
    page text NOT NULL DEFAULT '',
    since timestamptz NOT NULL,
    seen integer NOT NULL DEFAULT 1,
    missing integer NOT NULL DEFAULT 0,
    announced boolean NOT NULL DEFAULT false
);
