-- Each user's content settings: the server libraries their apps do not
-- show (libraries added later show), the genres whose titles are hidden
-- from them, and the hours they may use the server, as Jellyfin's
-- AccessSchedules: a day (one of Jellyfin's DynamicDayOfWeek names) and a
-- start and end hour from 0 to 24.
ALTER TABLE users
    ADD COLUMN hidden_libraries uuid[] NOT NULL DEFAULT '{}',
    ADD COLUMN blocked_genres text[] NOT NULL DEFAULT '{}' CHECK (cardinality(blocked_genres) <= 100),
    ADD COLUMN access_schedules jsonb NOT NULL DEFAULT '[]' CHECK (CASE WHEN jsonb_typeof(access_schedules) = 'array' THEN
        jsonb_array_length(access_schedules) <= 50 AND NOT jsonb_path_exists(access_schedules,
            '$[*] ? (!(exists(@.start) && exists(@.end) && @.start >= 0 && @.end <= 24 && @.start < @.end
                && (@.day == "Sunday" || @.day == "Monday" || @.day == "Tuesday" || @.day == "Wednesday"
                    || @.day == "Thursday" || @.day == "Friday" || @.day == "Saturday"
                    || @.day == "Everyday" || @.day == "Weekday" || @.day == "Weekend")))')
        ELSE false END);
