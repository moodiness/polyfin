package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// startWait bounds a server's start, its migrations included: the image's
// health check gives it 2 minutes, and a slow runner some more.
const startWait = 3 * time.Minute

// docker runs the Docker CLI, and returns its output trimmed; its error
// holds what the CLI printed on failure.
func docker(ctx context.Context, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		return "", fmt.Errorf("docker %s: %w: %s", args[0], err, message)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// makeMedia fills a new volume with the media files of the local folders
// the releases add (see seed.go): three home videos of two seconds and an
// album of two tagged songs, made by the candidate's FFmpeg, as root, the
// volume being root's.
func makeMedia(ctx context.Context, image, volume string) error {
	const script = `set -e
video() {
	mkdir -p "$(dirname "$1")"
	ffmpeg -nostdin -loglevel error -y -f lavfi -i "testsrc=size=160x90:rate=10:duration=2" \
		-f lavfi -i "sine=frequency=$2:duration=2" -c:v libx264 -preset ultrafast -pix_fmt yuv420p \
		-c:a aac -b:a 32k -shortest "$1"
}
song() {
	mkdir -p "$(dirname "$1")"
	ffmpeg -nostdin -loglevel error -y -f lavfi -i "sine=frequency=$2:duration=2" -ac 2 -c:a flac \
		-metadata artist="Tone Quartet" -metadata album_artist="Tone Quartet" -metadata album="Sine Studies" \
		-metadata title="$3" -metadata track="$4" -metadata date=2021 -metadata genre=Electronic "$1"
}
video "/media/videos/Garden Party/Garden Party.mp4" 440
video "/media/videos/School Play/School Play.mp4" 550
video "/media/videos/Beach Day/Beach Day.mp4" 660
song "/media/music/Tone Quartet/Sine Studies/01 First Light.flac" 330 "First Light" 1
song "/media/music/Tone Quartet/Sine Studies/02 Second Wind.flac" 440 "Second Wind" 2
`
	if _, err := docker(ctx, "volume", "create", volume); err != nil {
		return err
	}
	_, err := docker(ctx, "run", "--rm", "--user", "0", "--network", "none", "-v", volume+":/media",
		"--entrypoint", "sh", image, "-c", script)
	return err
}

// container is a server's container, and how to reach it.
type container struct {
	name string
	api  *api
}

// startServer starts image as the upgrade's server, on its database and
// with its data volume, as compose.yaml and compose.embedded.yaml run it,
// and waits for it to be healthy and its database ready.
func (u *upgrade) startServer(ctx context.Context, image, name string) (*container, error) {
	args := []string{"run", "-d", "--name", name, "-v", u.name + "-data:/data", "-v", u.media + ":/media:ro",
		"-p", "127.0.0.1::8096", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--stop-timeout", "60"}
	if u.database == embedded {
		args = append(args, "-e", "POLYFIN_DATABASE=embedded")
	} else {
		args = append(args, "--network", u.name, "-e", "POLYFIN_DATABASE_URL=postgresql://polyfin:polyfin@"+u.name+"-postgres:5432/polyfin")
	}
	if _, err := docker(ctx, append(args, image)...); err != nil {
		return nil, err
	}
	u.containers = append(u.containers, name)
	address, err := docker(ctx, "port", name, "8096/tcp")
	if err != nil {
		return nil, err
	}
	// The first line: Docker may list the port of IPv6 too.
	address, _, _ = strings.Cut(address, "\n")
	c := &container{name: name, api: newAPI("http://" + address)}
	deadline := time.Now().Add(startWait)
	for {
		state, err := docker(ctx, "inspect", "--format", "{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}", name)
		if err != nil {
			return nil, err
		}
		running, health, _ := strings.Cut(state, " ")
		if running != "running" {
			return nil, fmt.Errorf("the container stopped (%s) before it was healthy\n%s", running, u.logTail(ctx, name))
		}
		if health == "healthy" {
			var status struct {
				Database string `json:"database"`
			}
			if _, err := c.api.call(ctx, nobody, "GET", "/admin/api/status", nil, &status, 200); err == nil && status.Database == "ready" {
				return c, nil
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("not healthy with its database ready after %s (health: %s)\n%s", startWait, health, u.logTail(ctx, name))
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// stopServer stops a server as docker stop does an update, and checks
// that it stopped cleanly.
func (u *upgrade) stopServer(ctx context.Context, name string) error {
	if _, err := docker(ctx, "stop", name); err != nil {
		return err
	}
	code, err := docker(ctx, "inspect", "--format", "{{.State.ExitCode}}", name)
	if err != nil {
		return err
	}
	if code != "0" {
		return fmt.Errorf("it exited with code %s\n%s", code, u.logTail(ctx, name))
	}
	return nil
}

// logTail is the end of a container's log, for a failure to show.
func (u *upgrade) logTail(ctx context.Context, name string) string {
	var out bytes.Buffer
	cmd := exec.CommandContext(context.WithoutCancel(ctx), "docker", "logs", "--tail", "40", name)
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	return "The end of its log:\n" + strings.TrimRight(out.String(), "\n")
}

// setupCode is the setup code a new server logs.
func setupCode(ctx context.Context, name string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "logs", name)
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	_, after, found := strings.Cut(out.String(), "setup_code=")
	if !found {
		return "", errors.New("the server logged no setup code")
	}
	code, _, _ := strings.Cut(after, "\n")
	return strings.Trim(strings.TrimSpace(code), `"`), nil
}

// startPostgres starts the external database on the upgrade's network,
// as compose.yaml runs it, its data in memory: it outlives both servers.
func (u *upgrade) startPostgres(ctx context.Context) error {
	if _, err := docker(ctx, "network", "create", u.name); err != nil {
		return err
	}
	u.networks = append(u.networks, u.name)
	name := u.name + "-postgres"
	if _, err := docker(ctx, "run", "-d", "--name", name, "--network", u.name, "--tmpfs", "/var/lib/postgresql",
		"-e", "POSTGRES_USER=polyfin", "-e", "POSTGRES_PASSWORD=polyfin", u.postgres,
		"postgres", "-c", "shared_buffers=32MB", "-c", "max_connections=20", "-c", "max_locks_per_transaction=512",
		"-c", "max_worker_processes=2", "-c", "max_parallel_workers=0", "-c", "autovacuum_max_workers=1",
		"-c", "max_wal_size=256MB", "-c", "jit=off"); err != nil {
		return err
	}
	u.containers = append(u.containers, name)
	deadline := time.Now().Add(time.Minute)
	for {
		// The image's first start runs initdb, then restarts the server:
		// a query, rather than pg_isready, waits for the second.
		if _, err := docker(ctx, "exec", name, "psql", "-U", "polyfin", "-d", "polyfin", "-h", "127.0.0.1", "-c", "SELECT 1"); err == nil {
			return nil
		} else if time.Now().After(deadline) {
			return fmt.Errorf("PostgreSQL did not start: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// query runs a query on the upgrade's database with psql, unaligned,
// fields separated by |: on the external database in its container; on
// the embedded one, from a container of PostgreSQL's image in the
// server's network namespace, with the password and port the cluster
// keeps in the data volume, read as the server's user.
func (u *upgrade) query(ctx context.Context, server, sql string) (string, error) {
	if u.database == external {
		return docker(ctx, "exec", u.name+"-postgres", "psql", "-U", "polyfin", "-d", "polyfin", "-h", "127.0.0.1",
			"-v", "ON_ERROR_STOP=1", "-At", "-F", "|", "-c", sql)
	}
	const script = `PGPASSWORD="$(cat /data/postgresql/polyfin.password)" exec psql -h 127.0.0.1 \
		-p "$(cat /data/postgresql/polyfin.port)" -U polyfin -d polyfin -v ON_ERROR_STOP=1 -At -F '|' -c "$1"`
	return docker(ctx, "run", "--rm", "--network", "container:"+server, "--user", "65532:65532",
		"-v", u.name+"-data:/data:ro", "--entrypoint", "sh", u.postgres, "-c", script, "query", sql)
}

// cleanUp removes what the upgrade made, unless -keep.
func (u *upgrade) cleanUp() {
	if u.keep {
		return
	}
	ctx := context.Background()
	for _, name := range u.containers {
		_, _ = docker(ctx, "rm", "-f", "-v", name)
	}
	_, _ = docker(ctx, "volume", "rm", u.name+"-data")
	for _, name := range u.networks {
		_, _ = docker(ctx, "network", "rm", name)
	}
}
