-- A registration code may create several accounts: max_uses of them, 0
-- for no limit; uses counts those created, used_at and used_by tell the
-- last.
ALTER TABLE registration_codes
	ADD COLUMN uses integer NOT NULL DEFAULT 0,
	ADD COLUMN max_uses integer NOT NULL DEFAULT 1;
UPDATE registration_codes SET uses = 1 WHERE used_at IS NOT NULL;
