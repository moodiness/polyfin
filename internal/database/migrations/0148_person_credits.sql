-- A title described again takes itself off the people it no longer
-- credits: this index finds the people crediting a title.
CREATE INDEX items_person_credits ON items USING gin ((data->'credits')) WHERE kind = 'person';
