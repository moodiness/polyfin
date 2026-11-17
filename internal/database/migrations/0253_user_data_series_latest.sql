-- The series a user watches, with the latest date of each, read in index
-- order rather than sorted (userdata's WatchedSeries and PlayedSeries):
-- each series' episodes come latest first, ties by episode, and whether
-- each was played rides along. Next Up and Upcoming sorted every episode
-- row of the user at each call, 20,000 for a long history. The index
-- leads with the columns of user_data_series, which it replaces: the
-- episodes of some series are read through it too, and a row written
-- keeps the same number of indexes to update.
CREATE INDEX user_data_series_latest ON user_data (user_id, series_id, last_played_at DESC NULLS LAST, item_id) INCLUDE (played)
    WHERE series_id IS NOT NULL;
DROP INDEX user_data_series;
