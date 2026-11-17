-- confined tells a connection a member made, whose later reads reach
-- public addresses only, as their addons do: a Nuvio backend on a local
-- network is for administrators. A connection an administrator made, for
-- themselves or for a member, keeps its reach; any other is confined,
-- one whose maker's account is gone included, and so is a row that does
-- not say.
ALTER TABLE app_connections ADD COLUMN confined boolean NOT NULL DEFAULT true;

UPDATE app_connections c SET confined = false FROM users u WHERE u.id = c.connected_by AND u.is_administrator;
