-- Polyfin's metrics, for Prometheus, at /metrics: off by default. Turned
-- on, they need metrics_token, which Polyfin makes, sent as a bearer
-- token; it is stored sealed with POLYFIN_SECRET_KEY when it is set, as
-- the server's other keys are, and empty until metrics are first on.
ALTER TABLE settings
    ADD COLUMN metrics boolean NOT NULL DEFAULT false,
    ADD COLUMN metrics_token text NOT NULL DEFAULT '' CHECK (octet_length(metrics_token) <= 512 AND metrics_token ~ '^[ -~]*$');
