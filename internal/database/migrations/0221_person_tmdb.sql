-- Health counts the people looked up on TMDB (library.PeopleStatus): this
-- index finds them, the few people with TMDB's details, without reading
-- every title.
CREATE INDEX items_person_tmdb ON items (id) WHERE kind = 'person' AND data ? 'tmdb';
