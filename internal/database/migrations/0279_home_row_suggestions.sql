-- A home row may carry a note from the administrator who last changed it
-- (note_by, which their deletion forgets), shown under its name in the
-- web player. A row shown to chosen users may tell each of them once, as
-- it starts showing to them (notify), and a row of some kinds may leave
-- out the titles each user started or played (leave_played).
ALTER TABLE home_rows
    ADD COLUMN note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 300),
    ADD COLUMN note_by uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN notify boolean NOT NULL DEFAULT false,
    ADD COLUMN leave_played boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT home_rows_notify_check CHECK (NOT (notify AND everyone)),
    ADD CONSTRAINT home_rows_leave_played_check CHECK (NOT leave_played OR kind IN ('catalog', 'collection', 'new', 'watchlist', 'playlist'));

-- What each user of a row shown to chosen users was told of it and did
-- with it: when their own targets were told (told_at), when one of their
-- apps showed it (apps_at), and when they first opened the row or one of
-- its titles from the home page (seen_at). A user is told once: a row
-- here with told_at set is never told again.
CREATE TABLE home_row_notices (
    row_id uuid NOT NULL REFERENCES home_rows (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    told_at timestamptz,
    apps_at timestamptz,
    seen_at timestamptz,
    PRIMARY KEY (row_id, user_id)
);
CREATE INDEX home_row_notices_user ON home_row_notices (user_id);

-- The rows shown to chosen users before count as seen and told, so that
-- they show no New badge after the upgrade.
INSERT INTO home_row_notices (row_id, user_id, told_at, seen_at)
SELECT row_id, user_id, now(), now() FROM home_row_users;
