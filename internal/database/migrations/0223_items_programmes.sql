-- The hourly sweep of past programmes reads the programmes and replays
-- kept as items through this index, not every item; without Live TV it
-- is empty.
CREATE INDEX items_programmes ON items (kind) WHERE kind IN ('program', 'replay');
