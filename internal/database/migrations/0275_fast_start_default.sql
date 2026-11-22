-- Start remote plays faster becomes on by default. Servers still at the
-- old default, off, turn it on: nothing stored tells off apart from an
-- administrator's choice. A server that turned it on keeps it, and one
-- turned off from now on stays off.
ALTER TABLE settings
    ALTER COLUMN fast_start SET DEFAULT true;

UPDATE settings SET fast_start = true WHERE NOT fast_start;
