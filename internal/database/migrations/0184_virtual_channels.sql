-- Channels Polyfin airs itself, built from titles of the library (see
-- package virtualtv): a collection, a smart list, a series or titles
-- picked by hand, which source names. entries are the movies and episodes
-- they held when the channel was saved, frozen so that the guide stays
-- stable, with what the schedule and the guide need of each. The schedule
-- is computed from epoch, seed and the entries, and kept nowhere: shuffle
-- orders the entries anew each round, by seed. blocks are the daily time
-- blocks, each with its own source and entries, which the channel's own
-- entries fill around. number is the channel's number, NULL to follow the
-- other channels.
CREATE TABLE virtual_channels (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (name <> '' AND length(name) <= 64),
    number integer CHECK (number BETWEEN 1 AND 9999),
    enabled boolean NOT NULL DEFAULT true,
    source jsonb NOT NULL,
    entries jsonb NOT NULL,
    shuffle boolean NOT NULL DEFAULT false,
    seed bigint NOT NULL,
    blocks jsonb NOT NULL DEFAULT '[]',
    epoch timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
