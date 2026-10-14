-- The places database backups are sent to besides the backup folder, at
-- most 10. address is an S3-compatible storage's endpoint, an SFTP server's
-- host, or a WebDAV folder's address; port an SFTP server's port, 0 for
-- the other kinds; bucket an S3 bucket; folder an S3 prefix or an SFTP
-- folder, empty for the bucket's root or the user's home; region an S3
-- region, empty for the default one; username an S3 access key or the user
-- signing in. secret is an S3 secret key or a password, private_key an SFTP
-- private key, both sealed with POLYFIN_SECRET_KEY when it is set; host_key
-- the SFTP server's key saved at the first connection, in the
-- authorized_keys format. kept is how many backups are kept there.
CREATE TABLE backup_destinations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind text NOT NULL CHECK (kind IN ('s3', 'sftp', 'webdav')),
    address text NOT NULL CHECK (address <> '' AND octet_length(address) <= 2048),
    port integer NOT NULL DEFAULT 0 CHECK (port BETWEEN 0 AND 65535),
    bucket text NOT NULL DEFAULT '' CHECK (octet_length(bucket) <= 63),
    folder text NOT NULL DEFAULT '' CHECK (octet_length(folder) <= 512),
    region text NOT NULL DEFAULT '' CHECK (octet_length(region) <= 64),
    username text NOT NULL DEFAULT '' CHECK (octet_length(username) <= 256),
    secret text NOT NULL DEFAULT '',
    private_key text NOT NULL DEFAULT '',
    host_key text NOT NULL DEFAULT '',
    kept integer NOT NULL DEFAULT 7 CHECK (kept BETWEEN 1 AND 90),
    created_at timestamptz NOT NULL DEFAULT now(),
    -- The last upload: when it started, and its error, empty after a
    -- success; the newest backup there after it, by name, and when that
    -- backup was made.
    ran_at timestamptz,
    error text NOT NULL DEFAULT '',
    newest_file text NOT NULL DEFAULT '',
    newest_at timestamptz,
    CHECK ((kind = 's3') = (bucket <> '')),
    CHECK ((kind = 'sftp') = (port > 0))
);

-- The last restore of a backup from the admin app or a Jellyfin app, one
-- row once one has run: when it ran, the backup restored, the
-- administrator who asked, and its error, empty after a success. A restore
-- that succeeds writes it into the restored database.
CREATE TABLE backup_restore (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    restored_at timestamptz NOT NULL,
    file text NOT NULL,
    administrator text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT ''
);
