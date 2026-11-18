-- A playlist's name is bounded, as a collection's: the routes that name a
-- playlist refuse a longer one, and so does the table. Names already
-- longer are cut to the bound first, so that the check holds for every
-- row and later changes to those playlists are not refused for the name
-- they had. A watchlist that keeps the default name has none.
UPDATE playlists SET name = left(name, 500) WHERE char_length(name) > 500;
ALTER TABLE playlists ADD CONSTRAINT playlists_name_length CHECK (char_length(name) <= 500);
