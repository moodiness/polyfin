-- When each scheduled task runs, set by an administrator: a JSON array of
-- triggers, a task without a row keeping its default ones. The hours of
-- the settings become daily triggers only when they were not their
-- defaults: -1, none, and 4 for the backup hour.
CREATE TABLE task_triggers (
    task_key text PRIMARY KEY,
    triggers jsonb NOT NULL CHECK (jsonb_typeof(triggers) = 'array'),
    changed_at timestamptz NOT NULL DEFAULT now()
);

-- The last run of each scheduled task.
CREATE TABLE task_runs (
    task_key text PRIMARY KEY,
    started_at timestamptz NOT NULL,
    ended_at timestamptz NOT NULL,
    status text NOT NULL CHECK (status IN ('Completed', 'Failed', 'Cancelled')),
    error text NOT NULL DEFAULT ''
);

INSERT INTO task_triggers (task_key, triggers)
SELECT 'ReadCollections',
       jsonb_build_array(jsonb_build_object('type', 'daily', 'time', collection_read_hour * 60))
FROM settings
WHERE collection_read_hour >= 0;

INSERT INTO task_triggers (task_key, triggers)
SELECT 'EmptySourceCache',
       jsonb_build_array(jsonb_build_object('type', 'daily', 'time', source_cache_empty_hour * 60))
FROM settings
WHERE source_cache_empty_hour >= 0;

INSERT INTO task_triggers (task_key, triggers)
SELECT 'BackUpDatabase', jsonb_build_array(jsonb_build_object('type', 'daily', 'time', backup_hour * 60))
FROM settings
WHERE backup_hour <> 4;

ALTER TABLE settings
    DROP COLUMN collection_read_hour,
    DROP COLUMN source_cache_empty_hour,
    DROP COLUMN backup_hour;
