-- What saves learned of the items they left as they are, kept across
-- restarts (see library.savedRecords): fingerprint is that of the record
-- whose save left the item as it is at version, the item's xmin then,
-- which every write of the item changes. A save leaves out a record whose
-- fingerprint its item holds at its current version, instead of merging
-- it again after each start. Only records that a merge would leave as
-- they are whatever the catalog arrivals are have one.
CREATE TABLE item_records (
    item_id uuid PRIMARY KEY REFERENCES items (id) ON DELETE CASCADE,
    version xid NOT NULL,
    fingerprint bigint NOT NULL
);
