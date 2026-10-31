-- How each notification target paces its messages: at most per_minute
-- messages in any minute, NULL for no cap; and the plays and new episodes
-- grouped into one message every group_minutes minutes, NULL to send each
-- at once. Urgent events are never grouped.
ALTER TABLE notification_targets
    ADD COLUMN per_minute integer CHECK (per_minute BETWEEN 1 AND 60),
    ADD COLUMN group_minutes integer CHECK (group_minutes BETWEEN 1 AND 1440);

-- What came of each message sent to a target, for its Recent deliveries:
-- the event, as its type, its identifier and its title, how many events a
-- grouped message held, and its outcome: 'delivered', 'refused' (401, 403,
-- 404 or 410), 'rejected' (another 4xx), 'failed' (given up after
-- retries), 'dropped' (too many messages waited) or 'unreadable' (the
-- target's secret cannot be decrypted), with the status the target
-- answered and why it did not accept it, without its address or tokens.
-- Each target keeps its last 100, for 30 days.
CREATE TABLE notification_deliveries (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id uuid NOT NULL REFERENCES notification_targets ON DELETE CASCADE,
    event_id text NOT NULL,
    type text NOT NULL,
    title text NOT NULL,
    events integer NOT NULL DEFAULT 1 CHECK (events >= 1),
    outcome text NOT NULL CHECK (outcome IN ('delivered', 'refused', 'rejected', 'failed', 'dropped', 'unreadable')),
    status integer,
    error text NOT NULL DEFAULT '',
    attempts integer NOT NULL DEFAULT 0,
    queued_at timestamptz NOT NULL,
    at timestamptz NOT NULL
);

CREATE INDEX notification_deliveries_target ON notification_deliveries (target_id, at DESC);

-- The crashes of the server, as the crash file the next start found told
-- them: when it was written, and the first lines of its trace, redacted.
-- The last ten are kept; System › Health shows the last.
CREATE TABLE server_crashes (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    crashed_at timestamptz NOT NULL,
    trace text NOT NULL,
    found_at timestamptz NOT NULL
);

-- The server's targets and administrators' own that tell of health
-- problems tell of crashes too.
UPDATE notification_targets SET events = array_append(events, 'server_crashed')
    WHERE 'health_problem' = ANY (events) AND NOT 'server_crashed' = ANY (events);
