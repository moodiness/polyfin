package localfiles

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"path"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// A .strm file, as Jellyfin and Kodi read one, is a text file whose content
// is the address of a stream: a title of a movies or shows folder named
// as a video is, whose version plays that address as an addon's link is
// played (see stremio.Stream.Remote), through the analysis, the source
// cache, the relay and the conversion, and whose apps are sent to the
// address only as Send apps to the source lets them be sent to a server
// addon's link (see playback.Service.SendsToSource). Its first line that
// is neither blank nor a comment, as the #KODIPROP and #EXTINF lines Kodi
// and Jellyfin skip, is the address; the lines after it are not read.
//
// Only an http or https address is played. A file naming none, empty or
// naming a path, a relative address or another scheme, or larger than
// maxStrm, is left out of the scan, which logs why once until the file
// changes. A scan reads a file new or changed, by its size and
// modification time, so that a rescan plays the address the file names
// now; one it cannot read keeps the address read before.
// An address may hold a key, as an addon's link does: it is stored sealed
// with POLYFIN_SECRET_KEY when it is set, as the addons' addresses are
// (see secrets.Box.SealStored), so that neither a copy of the database
// nor a backup shows it. One the key cannot open is read again from its
// file at the next scan, and plays meanwhile for no one. Logs show only
// its scheme and host (see logs.Redact), and scans log a file's path,
// never what it holds.

// maxStrm bounds a .strm file: one address, with a few lines of Kodi's
// properties.
const maxStrm = 1024

// Why a .strm file names no address Polyfin plays, as the scan logs it.
const (
	strmEmpty    = "empty"
	strmTooLarge = "too_large"
	strmNotHTTP  = "not_http"
)

// strmFile reports whether name is a .strm file.
func strmFile(name string) bool { return strings.EqualFold(path.Ext(name), ".strm") }

// strmAddress reads the address a .strm file's content names (see the
// comment at the top of this file), else why it names none.
func strmAddress(content []byte) (address, reason string) {
	content = bytes.TrimPrefix(content, []byte("\uFEFF"))
	for line := range strings.Lines(string(content)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parsed, err := url.Parse(line)
		if err != nil || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) || parsed.Host == "" {
			return "", strmNotHTTP
		}
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		return parsed.String(), ""
	}
	return "", strmEmpty
}

// openStream returns the address a .strm file names as local_files stores
// it: sealed with the key, or as it is without one; empty when the key
// cannot open it.
func (s *Service) openStream(stored string) string {
	address, err := s.box.Open(stored)
	if err != nil {
		return ""
	}
	return address
}

// strmFilename is the file name a .strm file's stream bears: the file's
// own, with the extension of its address's file when it is a video's or a
// manifest's, which tells apps its container until it is analyzed, and
// none otherwise.
func strmFilename(rel, address string) string {
	name := stem(rel)
	if parsed, err := url.Parse(address); err == nil {
		extension := strings.ToLower(path.Ext(parsed.Path))
		if videoFile(extension) || extension == ".m3u8" || extension == ".mpd" {
			return name + extension
		}
	}
	return name
}

// readStreams reads the address of each .strm file a walk of a movies or
// shows folder listed: the one stored when the file is as it was, else
// the one it holds now. A file naming no address Polyfin plays is taken
// out of the listing, and logged unless it was, as it is, at the folder's
// last scan; one that cannot be read now is taken out too, its row kept
// as those of a folder the walk skipped are (see under).
func (s *Service) readStreams(ctx context.Context, folder Folder, list *listing, known map[string]stored) (map[string]string, error) {
	id := folder.Addon.ID
	s.mu.Lock()
	logged := s.badStrm[id]
	s.mu.Unlock()
	streams := map[string]string{}
	bad := map[string]found{}
	var unread []string
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(s.workers)
	for rel, f := range list.files {
		if !strmFile(rel) {
			continue
		}
		// The workers write these maps too.
		if old, ok := known[rel]; ok && old.stream != "" && old.size == f.size && old.modified.Equal(f.modified) {
			mu.Lock()
			streams[rel] = old.stream
			mu.Unlock()
			continue
		}
		if was, ok := logged[rel]; ok && was.size == f.size && was.modified.Equal(f.modified) {
			mu.Lock()
			bad[rel] = f
			mu.Unlock()
			continue
		}
		group.Go(func() error {
			address, reason, err := readStrm(groupCtx, list, rel, f)
			if err != nil && groupCtx.Err() != nil {
				return groupCtx.Err()
			}
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				s.logger.Warn("A .strm file cannot be read: it keeps the address read before", "folder", folder.Addon.LogName(), "path", rel,
					"error", err)
				unread = append(unread, rel)
			case reason != "":
				s.logger.Warn("A .strm file names no http or https address: it is left out", "folder", folder.Addon.LogName(), "path", rel,
					"reason", reason)
				bad[rel] = f
			default:
				streams[rel] = address
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	for rel := range bad {
		delete(list.files, rel)
	}
	for _, rel := range unread {
		delete(list.files, rel)
		list.skipped = append(list.skipped, rel)
	}
	s.mu.Lock()
	if len(bad) > 0 {
		s.badStrm[id] = bad
	} else {
		delete(s.badStrm, id)
	}
	s.mu.Unlock()
	return streams, nil
}

// readStrm reads the address the .strm file rel of a listing names, else
// why it names none.
func readStrm(ctx context.Context, list *listing, rel string, f found) (address, reason string, err error) {
	if f.size > maxStrm {
		return "", strmTooLarge, nil
	}
	readCtx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	file, err := list.open(readCtx, rel)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxStrm+1))
	if err != nil {
		return "", "", err
	}
	if len(content) > maxStrm {
		return "", strmTooLarge, nil
	}
	address, reason = strmAddress(content)
	return address, reason, nil
}
