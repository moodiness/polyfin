package accounts

// Conversions are the conversions a user's apps may have: Video converts
// the video, which burning subtitles in also takes; Audio, the audio.
// Playing a file as it is, or repackaged with its tracks copied, needs
// neither.
type Conversions struct {
	Video, Audio bool
}

// Conversions returns the conversions user may have: those their own
// permissions allow, while the server converts at all.
func (s *Store) Conversions(user User) Conversions {
	on := s.Settings().Transcoding
	return Conversions{Video: on && user.VideoTranscoding, Audio: on && user.AudioTranscoding}
}

// MayDownload reports whether user may download titles: their own
// permission allows it, and the server allows downloads.
func (s *Store) MayDownload(user User) bool {
	return s.Settings().Downloads && user.ContentDownloading
}

// PersonalAddonsAllowed reports whether user may add and use their own
// addons: their own permission allows it, and the server allows users' own
// addons. Otherwise their addons are kept, but not used.
func (settings Settings) PersonalAddonsAllowed(user User) bool {
	return settings.PersonalAddons && user.PersonalAddons
}
