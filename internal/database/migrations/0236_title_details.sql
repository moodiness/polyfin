-- The details TMDB gives of a movie or series with its credits: the names
-- of the networks a series aired on and of the companies that made the
-- title, each once, and its tagline, empty when TMDB has none, in the
-- row's language. NULL until they are read, which makes the row due: the
-- rows kept before are read again.
ALTER TABLE title_credits
    ADD COLUMN networks text[],
    ADD COLUMN companies text[],
    ADD COLUMN tagline text;
