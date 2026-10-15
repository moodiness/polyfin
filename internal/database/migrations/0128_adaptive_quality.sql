-- Whether the master playlist of converted video offers lower qualities,
-- which apps step down to on a weak connection: off, to apps outside the
-- local network (remote), or always.
ALTER TABLE settings
    ADD COLUMN adaptive_quality text NOT NULL DEFAULT 'off' CHECK (adaptive_quality IN ('off', 'remote', 'always'));
