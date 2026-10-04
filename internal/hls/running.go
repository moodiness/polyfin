package hls

// Running is an encoding under way, as the admin dashboard shows it.
type Running struct {
	Key Key
	// Live is set for the encoding of a channel, read as it comes.
	Live bool
	// Opened is set once its input is open: what follows is known then.
	Opened bool
	// Video is what the video is converted to, nil when it is copied.
	Video *VideoEncoding
	// AudioCodec is the encoder the audio is converted with, empty when
	// it is copied; AudioChannels and AudioBitrate, what it is converted
	// to.
	AudioCodec    string
	AudioChannels int
	AudioBitrate  int64
}

// Running lists the encodings under way, of files and channels; the
// recordings are left out. It never waits for an input being opened.
func (m *Manager) Running() []Running {
	m.mu.Lock()
	encodings := make([]*encoding, 0, len(m.encodings))
	for _, e := range m.encodings {
		encodings = append(encodings, e)
	}
	lives := make([]*live, 0, len(m.lives))
	for _, l := range m.lives {
		lives = append(lives, l)
	}
	m.mu.Unlock()

	running := make([]Running, 0, len(encodings)+len(lives))
	for _, e := range encodings {
		r := Running{Key: e.key}
		select {
		case <-e.opened:
			if e.err == nil {
				r.describe(e.remux)
			}
		default:
		}
		running = append(running, r)
	}
	for _, l := range lives {
		r := Running{Key: l.key, Live: true}
		// A live input being opened holds its lock: it is shown as such.
		if l.mu.TryLock() {
			if l.opened {
				r.describe(l.remux)
			}
			l.mu.Unlock()
		}
		running = append(running, r)
	}
	return running
}

func (r *Running) describe(remux Remux) {
	r.Opened = true
	if remux.Encode != nil {
		video := *remux.Encode
		r.Video = &video
	}
	r.AudioCodec, r.AudioChannels, r.AudioBitrate = remux.AudioCodec, remux.AudioChannels, remux.AudioBitrate
}

// Conversions counts the playbacks whose video is converted now, as the
// limit counts them (see admits), and tells the limit, 0 for none.
func (m *Manager) Conversions() (running, limit int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	type playback struct{ user, version string }
	converting := map[playback]bool{}
	for k := range m.encodings {
		if k.Converts {
			converting[playback{k.User, k.Version}] = true
		}
	}
	for k := range m.lives {
		if k.Converts {
			converting[playback{k.User, k.Version}] = true
		}
	}
	if m.conversions != nil {
		limit = max(m.conversions(), 0)
	}
	return len(converting), limit
}
