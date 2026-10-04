-- Each user's parental control, as Jellyfin's user policy holds it: the
-- highest rating score (and subscore at that score) the user may reach,
-- none for no limit, and the kinds of items (Jellyfin's UnratedItem names)
-- hidden from the user when they have no rating.
ALTER TABLE users
    ADD COLUMN max_parental_rating integer,
    ADD COLUMN max_parental_sub_rating integer,
    ADD COLUMN block_unrated_items text[] NOT NULL DEFAULT '{}';
