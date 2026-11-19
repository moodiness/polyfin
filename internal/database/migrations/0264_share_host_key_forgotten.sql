-- An SFTP folder's host key is saved without being shown only at the
-- folder's first connection. Once the folder moved to another server,
-- share_host_key_forgotten is set: the next key offered is refused and kept
-- in share_offered_key until an administrator accepts it by its
-- fingerprint, as a key that changed is.
ALTER TABLE local_folders
    ADD COLUMN share_host_key_forgotten boolean NOT NULL DEFAULT false;

-- An SFTP folder without a key may have moved to another server since its
-- last connection: nothing tells it from one never connected, so its next
-- key is shown to be accepted too.
UPDATE local_folders f SET share_host_key_forgotten = true
FROM addons a
WHERE a.id = f.addon_id AND lower(a.manifest_url) LIKE 'sftp://%' AND f.share_host_key = '';
