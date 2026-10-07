-- Whether item details describe the versions not analyzed yet from
-- RemuxDB, off by default as it sends the titles' IMDb ids there, and the
-- address of the RemuxDB server asked.
ALTER TABLE settings
    ADD COLUMN remuxdb boolean NOT NULL DEFAULT false,
    ADD COLUMN remuxdb_url text NOT NULL DEFAULT 'https://remuxdb.1632022.xyz'
        CHECK ((remuxdb_url LIKE 'http://%' OR remuxdb_url LIKE 'https://%') AND octet_length(remuxdb_url) <= 512);
