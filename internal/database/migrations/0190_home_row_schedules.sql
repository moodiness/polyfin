-- When each home row shows, in the server's time zone (see package
-- homerows): from start_date to end_date, both included, on its days of
-- the week (0 is Sunday; none is every day), from start_hour to end_hour
-- (end_hour before start_hour passes midnight; none is all day). A null
-- date leaves that side open. A row that is also a library follows its
-- dates only.
ALTER TABLE home_rows
    ADD COLUMN start_date date,
    ADD COLUMN end_date date,
    ADD COLUMN days smallint[] NOT NULL DEFAULT '{}' CHECK (days <@ '{0,1,2,3,4,5,6}' AND cardinality(days) <= 7),
    ADD COLUMN start_hour smallint CHECK (start_hour BETWEEN 0 AND 23),
    ADD COLUMN end_hour smallint CHECK (end_hour BETWEEN 1 AND 24),
    ADD CHECK (start_date <= end_date),
    ADD CHECK ((start_hour IS NULL) = (end_hour IS NULL)),
    ADD CHECK (start_hour <> end_hour);
