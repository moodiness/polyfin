-- The GPU methods of every system Polyfin runs on: VideoToolbox on macOS,
-- Intel's QSV and AMD's AMF on Windows, beside NVIDIA's and VAAPI. Each
-- system offers its own; a backup restored on another system keeps its
-- choice, which reads as auto there.
ALTER TABLE settings
    DROP CONSTRAINT settings_hardware_acceleration_check,
    ADD CONSTRAINT settings_hardware_acceleration_check
        CHECK (hardware_acceleration IN ('auto', 'nvenc', 'vaapi', 'videotoolbox', 'qsv', 'amf', 'none'));
