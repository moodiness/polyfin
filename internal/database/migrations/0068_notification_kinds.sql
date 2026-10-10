-- The SMTP server email notifications go through, set by an administrator:
-- its host, empty for none, which leaves email targets out; its port; how
-- the connection is secured, 'starttls' upgrading a plain connection,
-- 'tls' from the start, or 'none'; the user and password it is signed in
-- with, the password sealed with POLYFIN_SECRET_KEY; and the sender's
-- address and name.
ALTER TABLE settings
    ADD COLUMN smtp_host text NOT NULL DEFAULT '' CHECK (octet_length(smtp_host) <= 253 AND smtp_host ~ '^[!-~]*$'),
    ADD COLUMN smtp_port integer NOT NULL DEFAULT 587 CHECK (smtp_port BETWEEN 1 AND 65535),
    ADD COLUMN smtp_security text NOT NULL DEFAULT 'starttls' CHECK (smtp_security IN ('starttls', 'tls', 'none')),
    ADD COLUMN smtp_user text NOT NULL DEFAULT '' CHECK (octet_length(smtp_user) <= 256),
    ADD COLUMN smtp_password text NOT NULL DEFAULT '',
    ADD COLUMN smtp_from text NOT NULL DEFAULT '' CHECK (octet_length(smtp_from) <= 254),
    ADD COLUMN smtp_from_name text NOT NULL DEFAULT '' CHECK (octet_length(smtp_from_name) <= 128);

-- Four more kinds of targets:
-- - 'email' keeps its recipient's address in address, and no secret;
-- - 'telegram' keeps its chat in topic, its bot token in secret, and no
--   address: the Bot API's is Polyfin's;
-- - 'gotify' keeps its server in address, and its application token in
--   secret;
-- - 'pushover' keeps its user key and its application token in secret, on
--   two lines, and no address.
ALTER TABLE notification_targets
    DROP CONSTRAINT notification_targets_kind_check,
    DROP CONSTRAINT notification_targets_address_check,
    DROP CONSTRAINT notification_targets_check,
    DROP CONSTRAINT notification_targets_check1,
    ADD CONSTRAINT notification_targets_kind_check
        CHECK (kind IN ('webhook', 'discord', 'ntfy', 'email', 'telegram', 'gotify', 'pushover')),
    ADD CONSTRAINT notification_targets_address_check CHECK (CASE kind
        WHEN 'email' THEN address LIKE '%_@_%'
        WHEN 'telegram' THEN address = ''
        WHEN 'pushover' THEN address = ''
        ELSE address LIKE 'http://%' OR address LIKE 'https://%' END),
    ADD CONSTRAINT notification_targets_topic_check CHECK ((kind IN ('ntfy', 'telegram')) = (topic <> '')),
    ADD CONSTRAINT notification_targets_secret_check CHECK (kind IN ('ntfy', 'email') OR secret <> '');
