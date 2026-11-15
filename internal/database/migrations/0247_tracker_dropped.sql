-- The series each user dropped on a tracking service whose history they
-- import, as its last read of the service's dropped shows found them.
-- dropped_at is the date the service gives for the drop, NULL when it
-- gives none; seen_at is when an import first found the series dropped
-- there. A series dropped on any service leaves that user's Next Up,
-- Upcoming and Continue Watching, until no service lists it any more or
-- one of its episodes is played after the later of those dates (see
-- userdata.Store.Dropped). The rows of a service go when its history
-- import is turned off or it is disconnected.
CREATE TABLE tracker_dropped (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    service text NOT NULL CHECK (service IN ('trakt', 'simkl', 'mdblist', 'anilist', 'myanimelist')),
    series_id uuid NOT NULL,
    dropped_at timestamptz,
    seen_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, service, series_id)
);

-- Where each service's dropped shows stood at the last read, as its
-- activity timestamps tell: they are read whole again once it changes.
-- Empty reads them at the next import.
ALTER TABLE tracking_imports ADD COLUMN dropped_cursor text NOT NULL DEFAULT '';
