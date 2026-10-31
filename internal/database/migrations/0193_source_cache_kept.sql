-- The source cache keeps its blocks across restarts. source_cache_preset
-- sets cache_size_gb, source_cache_days and source_cache_credit_days, and
-- preload_whole_files, but for 'custom': 'off' keeps nothing past a
-- restart, as before. source_cache_days is how long a block is kept once
-- last read, 0 for nothing kept across restarts; source_cache_credit_days
-- how much longer for each time a title was played again. A server whose
-- cache size is not the default keeps it, as a custom preset.
-- preload_next_episodes, off by default, reads ahead the next
-- preload_episodes episodes of the series users watch lately, at
-- preload_mbps at most, while the disk keeps preload_reserve_gb free.
ALTER TABLE settings
    ADD COLUMN source_cache_preset text NOT NULL DEFAULT 'small'
        CHECK (source_cache_preset IN ('off', 'small', 'everyday', 'large', 'custom')),
    ADD COLUMN source_cache_days integer NOT NULL DEFAULT 3 CHECK (source_cache_days BETWEEN 0 AND 365),
    ADD COLUMN source_cache_credit_days integer NOT NULL DEFAULT 1 CHECK (source_cache_credit_days BETWEEN 0 AND 30),
    ADD COLUMN preload_next_episodes boolean NOT NULL DEFAULT false,
    ADD COLUMN preload_episodes integer NOT NULL DEFAULT 1 CHECK (preload_episodes BETWEEN 1 AND 3),
    ADD COLUMN preload_mbps integer NOT NULL DEFAULT 20 CHECK (preload_mbps BETWEEN 1 AND 1000),
    ADD COLUMN preload_reserve_gb integer NOT NULL DEFAULT 20 CHECK (preload_reserve_gb BETWEEN 1 AND 1000),
    ADD COLUMN preload_whole_files boolean NOT NULL DEFAULT false;

UPDATE settings SET source_cache_preset = 'custom' WHERE cache_size_gb <> 10;
