// Package tools downloads FFmpeg and ffprobe 9.0 into the data folder of
// Polyfin Desktop at its first start: the builds of the Docker image for
// Windows and Linux, and a build made for macOS, each archive checked
// against the checksum pinned here before anything in it is used. The
// server, in Docker as in Polyfin Desktop, downloads the same way the
// FFmpeg for NVIDIA drivers older than 610 when NVENC needs it (see
// OlderDriver).
package tools

import (
	"archive/tar"
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ulikunitz/xz"
)

// ManifestVariable names a manifest file, JSON as Manifest, that replaces
// the pinned downloads: for development and tests against a local fake.
const ManifestVariable = "POLYFIN_DESKTOP_TOOLS"

// ErrChecksum is a download whose checksum or size is not the pinned one.
var ErrChecksum = errors.New("the download does not match its pinned checksum")

// Manifest is what to download for one system: archives, and where
// ffmpeg and ffprobe are once they are unpacked.
type Manifest struct {
	// Version names the folder the tools are unpacked in, tools/ffmpeg-<Version>.
	Version string `json:"version"`
	Files   []File `json:"files"`
	// FFmpeg and FFprobe are the executables' paths in that folder, with
	// slashes.
	FFmpeg  string `json:"ffmpeg"`
	FFprobe string `json:"ffprobe"`
}

// File is an archive to download and unpack.
type File struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	// Format is "zip" or "tar.xz".
	Format string `json:"format"`
	// Strip is the number of leading folders left out of the members'
	// paths, Into the folder they go in, and Keep the members unpacked:
	// a path, or a folder with a trailing slash; empty keeps them all.
	Strip int      `json:"strip"`
	Into  string   `json:"into"`
	Keep  []string `json:"keep"`
}

// Paths are the executables, once downloaded and unpacked.
type Paths struct {
	FFmpeg, FFprobe string
}

// Progress is told the bytes downloaded so far, of total.
type Progress func(done, total int64)

// btbn is the release of the builds the Docker image takes (Dockerfile):
// the shared builds, whose ffmpeg and ffprobe load the same libraries.
const btbn = "https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-09-30-13-08/ffmpeg-n9.0.2-17-g2a571b6068-"

// macOS has no build there: these are static builds made for macOS, with
// VideoToolbox, one archive per program.
const macos = "https://ffmpeg.martin-riedl.de/download/macos/"

// pinned are the downloads of each system, GOOS/GOARCH.
var pinned = map[string]Manifest{
	"linux/amd64": {Version: "9.0.2", FFmpeg: "bin/ffmpeg", FFprobe: "bin/ffprobe", Files: []File{{
		URL: btbn + "linux64-gpl-shared-9.0.tar.xz", Size: 68653636, Format: "tar.xz", Strip: 1,
		SHA256: "01a9764d0b5364b66cfeb4617557c64321b0e232e172ec73f0a701c5f7694326",
		Keep:   []string{"bin/ffmpeg", "bin/ffprobe", "lib/"},
	}}},
	"linux/arm64": {Version: "9.0.2", FFmpeg: "bin/ffmpeg", FFprobe: "bin/ffprobe", Files: []File{{
		URL: btbn + "linuxarm64-gpl-shared-9.0.tar.xz", Size: 57679524, Format: "tar.xz", Strip: 1,
		SHA256: "706ccdbc8b0537646345532713a9b6750c0a6258cc001ec1c0379754f0ae730a",
		Keep:   []string{"bin/ffmpeg", "bin/ffprobe", "lib/"},
	}}},
	"windows/amd64": {Version: "9.0.2", FFmpeg: "bin/ffmpeg.exe", FFprobe: "bin/ffprobe.exe", Files: []File{{
		URL: btbn + "win64-gpl-shared-9.0.zip", Size: 86321582, Format: "zip", Strip: 1,
		SHA256: "3da6c7b60bb9ccd73ec5b5e815ba804a0879eb362ba0e3beebce50174c022696",
		Keep:   []string{"bin/"},
	}}},
	"windows/arm64": {Version: "9.0.2", FFmpeg: "bin/ffmpeg.exe", FFprobe: "bin/ffprobe.exe", Files: []File{{
		URL: btbn + "winarm64-gpl-shared-9.0.zip", Size: 60050693, Format: "zip", Strip: 1,
		SHA256: "92460cf103340012d41413b557669902538f040e0c953abc1d719750dcd5f8a8",
		Keep:   []string{"bin/"},
	}}},
	"darwin/arm64": {Version: "9.0.2", FFmpeg: "bin/ffmpeg", FFprobe: "bin/ffprobe", Files: []File{
		{URL: macos + "arm64/1789931890_9.0.2/ffmpeg.zip", Size: 28395699, Format: "zip", Into: "bin", Keep: []string{"ffmpeg"},
			SHA256: "c8ed4c4e6978a03c485edbfe4e0a5dc2380f8a30bba5150531b31b094492d924"},
		{URL: macos + "arm64/1789931890_9.0.2/ffprobe.zip", Size: 28317701, Format: "zip", Into: "bin", Keep: []string{"ffprobe"},
			SHA256: "fcbe839537485eaee7a7a8bc5cbc0f90d53617e80943e8a5b2e31cb851197ea6"},
	}},
	"darwin/amd64": {Version: "9.0.2", FFmpeg: "bin/ffmpeg", FFprobe: "bin/ffprobe", Files: []File{
		{URL: macos + "amd64/1789931006_9.0.2/ffmpeg.zip", Size: 33816391, Format: "zip", Into: "bin", Keep: []string{"ffmpeg"},
			SHA256: "7c6b4125b191cbf773832dc51f424cf2b6bb7da43007d1e066f95909e47cacd4"},
		{URL: macos + "amd64/1789931006_9.0.2/ffprobe.zip", Size: 33719233, Format: "zip", Into: "bin", Keep: []string{"ffprobe"},
			SHA256: "2322438ed2f6319a691291b247d09c69dcaa3a982460d1f269a7e1af335cfdfd"},
	}},
}

// Pinned is the manifest of goos/goarch.
func Pinned(goos, goarch string) (Manifest, bool) {
	m, ok := pinned[goos+"/"+goarch]
	return m, ok
}

// Load reads a manifest file.
func Load(file string) (Manifest, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("read the tools manifest %s: %w", file, err)
	}
	return m, m.validate()
}

func (m Manifest) validate() error {
	if m.Version == "" || strings.ContainsAny(m.Version, `/\`) || m.FFmpeg == "" || m.FFprobe == "" || len(m.Files) == 0 {
		return errors.New("the tools manifest needs a version, ffmpeg, ffprobe and files")
	}
	for _, f := range m.Files {
		if len(f.SHA256) != sha256.Size*2 || (f.Format != "zip" && f.Format != "tar.xz") {
			return fmt.Errorf("the tools manifest's file %s needs a SHA-256 and a format, zip or tar.xz", f.URL)
		}
	}
	return nil
}

// Tools are the downloads into a tools folder.
type Tools struct {
	// Dir is the tools folder of the data folder.
	Dir      string
	Manifest Manifest
	Client   *http.Client
}

// folder is where the manifest's tools are unpacked.
func (t Tools) folder() string { return filepath.Join(t.Dir, "ffmpeg-"+t.Manifest.Version) }

func (t Tools) paths() Paths {
	return Paths{FFmpeg: filepath.Join(t.folder(), filepath.FromSlash(t.Manifest.FFmpeg)),
		FFprobe: filepath.Join(t.folder(), filepath.FromSlash(t.Manifest.FFprobe))}
}

// marker records, in the unpacked folder, the checksums of the archives
// it was unpacked from: a folder whose marker differs is unpacked again.
const marker = ".polyfin-tools"

func (t Tools) markerContent() []byte {
	var lines []string
	for _, f := range t.Manifest.Files {
		lines = append(lines, strings.ToLower(f.SHA256))
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// Ready returns the executables when they were downloaded already.
func (t Tools) Ready() (Paths, bool) {
	content, err := os.ReadFile(filepath.Join(t.folder(), marker))
	if err != nil || string(content) != string(t.markerContent()) {
		return Paths{}, false
	}
	paths := t.paths()
	for _, p := range []string{paths.FFmpeg, paths.FFprobe} {
		if _, err := os.Stat(p); err != nil {
			return Paths{}, false
		}
	}
	return paths, true
}

// Ensure returns the executables, downloading and unpacking them first
// when they are not there yet. Each archive is checked against its pinned
// checksum and size before it is unpacked: one that differs is removed and
// refused with ErrChecksum. Nothing is in place until every archive is
// unpacked; older versions are removed then.
func (t Tools) Ensure(ctx context.Context, progress Progress) (Paths, error) {
	if paths, ok := t.Ready(); ok {
		return paths, nil
	}
	if err := t.Manifest.validate(); err != nil {
		return Paths{}, err
	}
	downloads := filepath.Join(t.Dir, ".downloads")
	if err := os.MkdirAll(downloads, 0o700); err != nil {
		return Paths{}, err
	}
	defer os.RemoveAll(downloads)
	var total, done int64
	for _, f := range t.Manifest.Files {
		total += f.Size
	}
	report := func(n int64) {
		if progress != nil {
			progress(done+n, total)
		}
	}
	building := t.folder() + ".new"
	if err := os.RemoveAll(building); err != nil {
		return Paths{}, err
	}
	defer os.RemoveAll(building)
	for i, f := range t.Manifest.Files {
		archive := filepath.Join(downloads, fmt.Sprintf("%d.%s", i, f.Format))
		if err := t.download(ctx, f, archive, report); err != nil {
			return Paths{}, err
		}
		done += f.Size
		if err := unpack(archive, f, building); err != nil {
			return Paths{}, fmt.Errorf("unpack %s: %w", f.URL, err)
		}
	}
	for _, p := range []string{t.Manifest.FFmpeg, t.Manifest.FFprobe} {
		executable := filepath.Join(building, filepath.FromSlash(p))
		if err := os.Chmod(executable, 0o755); err != nil {
			return Paths{}, fmt.Errorf("the downloads lack %s: %w", p, err)
		}
	}
	if err := os.WriteFile(filepath.Join(building, marker), t.markerContent(), 0o600); err != nil {
		return Paths{}, err
	}
	if err := os.RemoveAll(t.folder()); err != nil {
		return Paths{}, err
	}
	if err := os.Rename(building, t.folder()); err != nil {
		return Paths{}, err
	}
	t.removeOthers()
	return t.paths(), nil
}

// removeOthers removes the folders of other versions.
func (t Tools) removeOthers() {
	entries, err := os.ReadDir(t.Dir)
	if err != nil {
		return
	}
	keep := filepath.Base(t.folder())
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "ffmpeg-") && entry.Name() != keep {
			_ = os.RemoveAll(filepath.Join(t.Dir, entry.Name()))
		}
	}
}

// download writes f to file, checking its size and checksum as it goes.
func (t Tools) download(ctx context.Context, f File, file string, report func(int64)) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return err
	}
	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download %s: %w", f.URL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: status %d", f.URL, response.StatusCode)
	}
	out, err := os.OpenFile(file, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	// One byte more than pinned is enough to tell a larger file.
	body := io.LimitReader(response.Body, f.Size+1)
	written, err := io.Copy(io.MultiWriter(out, hash), &counter{r: body, report: report})
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(file)
		return fmt.Errorf("download %s: %w", f.URL, err)
	}
	if written != f.Size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), f.SHA256) {
		os.Remove(file)
		return fmt.Errorf("%w: %s", ErrChecksum, f.URL)
	}
	return nil
}

// counter reports the bytes read through it.
type counter struct {
	r      io.Reader
	n      int64
	report func(int64)
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	c.report(c.n)
	return n, err
}

// target is where member goes in dir, false when f leaves it out. A path
// that would leave dir is refused.
func target(f File, member, dir string) (string, bool, error) {
	member = strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(member, `\`, "/")), "/")
	parts := strings.Split(member, "/")
	if len(parts) <= f.Strip {
		return "", false, nil
	}
	rest := strings.Join(parts[f.Strip:], "/")
	if len(f.Keep) > 0 && !slices.ContainsFunc(f.Keep, func(keep string) bool {
		return rest == keep || (strings.HasSuffix(keep, "/") && strings.HasPrefix(rest, keep))
	}) {
		return "", false, nil
	}
	out := filepath.Join(dir, filepath.FromSlash(f.Into), filepath.FromSlash(rest))
	if !strings.HasPrefix(out, filepath.Clean(dir)+string(filepath.Separator)) {
		return "", false, fmt.Errorf("the member %s leaves the folder", member)
	}
	return out, true, nil
}

func unpack(archive string, f File, dir string) error {
	if f.Format == "zip" {
		return unzip(archive, f, dir)
	}
	return untar(archive, f, dir)
}

func unzip(archive string, f File, dir string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, member := range reader.File {
		if member.FileInfo().IsDir() {
			continue
		}
		out, ok, err := target(f, member.Name, dir)
		if err != nil || !ok {
			if err != nil {
				return err
			}
			continue
		}
		in, err := member.Open()
		if err != nil {
			return err
		}
		err = write(out, in, member.Mode().Perm())
		in.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func untar(archive string, f File, dir string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	decompressed, err := xz.NewReader(file)
	if err != nil {
		return err
	}
	reader := tar.NewReader(decompressed)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		out, ok, err := target(f, header.Name, dir)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		switch header.Typeflag {
		case tar.TypeReg:
			if err := write(out, reader, header.FileInfo().Mode().Perm()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// The libraries' names link to their versioned files, beside
			// them.
			if strings.Contains(header.Linkname, "/") || strings.Contains(header.Linkname, `\`) {
				return fmt.Errorf("the link %s leaves its folder", header.Name)
			}
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, out); err != nil {
				return err
			}
		}
	}
}

func write(out string, in io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o644
	}
	file, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(file, in)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}
