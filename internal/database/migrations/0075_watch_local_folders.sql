-- Whether the local folders mounted in the container are watched for
-- changes, each scanned again a few seconds after its files stop changing.
ALTER TABLE settings
    ADD COLUMN watch_local_folders boolean NOT NULL DEFAULT true;
