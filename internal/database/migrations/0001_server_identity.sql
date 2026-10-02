-- Jellyfin clients recognise a server by this identifier, whatever address
-- they reach it at. It is generated once and must never change.
CREATE TABLE server_identity (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO server_identity (id) VALUES (gen_random_uuid());
