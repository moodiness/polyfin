-- The size of the admin app for each user: compact, the root at 85 % on
-- screens from 48rem up, or large, the root at 100 % on every screen.
-- Compact for every user, so that nothing changes until they choose.
ALTER TABLE users ADD COLUMN interface_size text NOT NULL DEFAULT 'compact' CHECK (interface_size IN ('compact', 'large'));
