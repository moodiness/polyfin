-- Silo's apps sign in and play through Silo's API under /api/v2 (see
-- package silo), off by default.
ALTER TABLE settings
    ADD COLUMN silo_apps boolean NOT NULL DEFAULT false;

-- The numbers Silo's apps know libraries and versions by: they take only
-- positive 32-bit integers there. Each identifier is given one the first
-- time an answer names it, and keeps it.
CREATE TABLE silo_numbers (
    number integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    id uuid NOT NULL UNIQUE
);
