-- The watchlist of a tracking service that tells when it last changed,
-- MDBList's, is read only once it changed: watchlist_cursor is that time
-- as the last import read it, for the connection of cursor_for. And
-- watchlist_full tells that the service refused the last title sent to the
-- watchlist as it is full, Trakt's, until a change goes through.
ALTER TABLE tracking_imports
    ADD COLUMN watchlist_cursor text NOT NULL DEFAULT '',
    ADD COLUMN watchlist_full boolean NOT NULL DEFAULT false;
