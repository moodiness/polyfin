-- How the server converts video and audio, as Jellyfin's Transcoding page
-- sets it: the encoder preset (auto lets Polyfin choose), the quality
-- factors of H.264 and HEVC (0 aims for the bitrate alone), HEVC for the
-- apps listing it first, the GPU (empty follows POLYFIN_HWACCEL) and the
-- codecs it decodes, tone mapping, its curve, peak in nits (0 keeps the
-- video's) and desaturation, the deinterlacer and its frame doubling, the
-- stereo downmix and its boost, the most audio channels (0 for no limit)
-- and the audio bitrate per channel in kb/s (0 keeps Polyfin's), FFmpeg's
-- threads (0 lets it choose) and how many segments a remux makes ahead of
-- the player. The defaults are what Polyfin did before.
ALTER TABLE settings
    ADD COLUMN encoder_preset text NOT NULL DEFAULT 'auto'
        CHECK (encoder_preset IN ('auto', 'veryslow', 'slower', 'slow', 'medium', 'fast', 'faster', 'veryfast', 'superfast', 'ultrafast')),
    ADD COLUMN h264_quality integer NOT NULL DEFAULT 0 CHECK (h264_quality BETWEEN 0 AND 51),
    ADD COLUMN hevc_quality integer NOT NULL DEFAULT 0 CHECK (hevc_quality BETWEEN 0 AND 51),
    ADD COLUMN allow_hevc_encoding boolean NOT NULL DEFAULT false,
    ADD COLUMN hardware_acceleration text NOT NULL DEFAULT '' CHECK (hardware_acceleration IN ('', 'auto', 'nvenc', 'vaapi', 'none')),
    ADD COLUMN hardware_decoding_codecs text[] NOT NULL DEFAULT '{h264,hevc,hevc_10bit,vp9,av1,mpeg2video,vc1}'
        CHECK (hardware_decoding_codecs <@ '{h264,hevc,hevc_10bit,vp9,av1,mpeg2video,vc1}'),
    ADD COLUMN tone_mapping boolean NOT NULL DEFAULT true,
    ADD COLUMN tone_mapping_algorithm text NOT NULL DEFAULT 'auto'
        CHECK (tone_mapping_algorithm IN ('auto', 'bt2390', 'hable', 'reinhard', 'mobius', 'clip', 'linear')),
    ADD COLUMN tone_mapping_peak integer NOT NULL DEFAULT 0 CHECK (tone_mapping_peak = 0 OR tone_mapping_peak BETWEEN 100 AND 10000),
    ADD COLUMN tone_mapping_desat double precision NOT NULL DEFAULT 0 CHECK (tone_mapping_desat BETWEEN 0 AND 10),
    ADD COLUMN deinterlace_method text NOT NULL DEFAULT 'yadif' CHECK (deinterlace_method IN ('yadif', 'bwdif')),
    ADD COLUMN deinterlace_double_rate boolean NOT NULL DEFAULT false,
    ADD COLUMN downmix_algorithm text NOT NULL DEFAULT 'None'
        CHECK (downmix_algorithm IN ('None', 'Dave750', 'NightmodeDialogue', 'Rfc7845', 'Ac4')),
    ADD COLUMN downmix_boost double precision NOT NULL DEFAULT 1 CHECK (downmix_boost BETWEEN 0.5 AND 3),
    ADD COLUMN max_audio_channels integer NOT NULL DEFAULT 0 CHECK (max_audio_channels IN (0, 1, 2, 6)),
    ADD COLUMN audio_bitrate_per_channel integer NOT NULL DEFAULT 0
        CHECK (audio_bitrate_per_channel = 0 OR audio_bitrate_per_channel BETWEEN 32 AND 320),
    ADD COLUMN encoding_threads integer NOT NULL DEFAULT 0 CHECK (encoding_threads BETWEEN 0 AND 64),
    ADD COLUMN ahead_segments integer NOT NULL DEFAULT 10 CHECK (ahead_segments BETWEEN 1 AND 60);
