#!/bin/sh
# The aio image image-check built, polyfin-local:aio, run as compose.aio.yaml
# runs it: read-only, without capabilities, with a named volume and no
# PostgreSQL of its own. It becomes healthy; docker stop stops Polyfin, then
# its database cleanly, which then starts without recovery; after docker
# kill, the database recovers and Polyfin becomes healthy again.
set -eu

name=polyfin-aio-health
volume=polyfin-aio-health
image=${1:-polyfin-local:aio}

cleanup() {
	docker rm -f "$name" >/dev/null 2>&1 || true
	docker volume rm "$volume" >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup

fail() {
	echo "image-health-aio: $*"
	docker logs "$name" 2>&1 | tail -n 60 || true
	docker exec "$name" cat /data/log/postgresql.log 2>/dev/null | tail -n 30 || true
	exit 1
}

# healthy waits for the container to become healthy, within the image's
# start period of 2 minutes.
healthy() {
	status=starting
	tries=0
	while [ "$status" = starting ] && [ $tries -lt 60 ]; do
		sleep 2
		tries=$((tries + 1))
		status="$(docker inspect --format '{{if .State.Running}}{{.State.Health.Status}}{{else}}exited{{end}}' "$name")"
	done
	echo "image-health-aio: $status after $((tries * 2)) seconds ($1)"
	[ "$status" = healthy ] || fail "not healthy ($1)"
}

# postgresql_log is the database's log, read through the volume.
postgresql_log() {
	docker run --rm --network none -v "$volume:/data:ro" --entrypoint cat "$image" /data/log/postgresql.log
}

docker volume create "$volume" >/dev/null
docker run -d --name "$name" --read-only --cap-drop ALL --security-opt no-new-privileges \
	--stop-timeout 60 -v "$volume:/data" "$image" >/dev/null
healthy "first start"

docker stop "$name" >/dev/null
code="$(docker inspect --format '{{.State.ExitCode}}' "$name")"
[ "$code" = 0 ] || fail "docker stop: exit code $code"
docker logs "$name" 2>&1 | grep -q "The database stopped" || fail "docker stop: the database was not stopped"
postgresql_log | grep -q "database system is shut down" || fail "docker stop: no clean shutdown in the database's log"
before="$(postgresql_log | grep -c "redo starts" || true)"

docker start "$name" >/dev/null
healthy "start after docker stop"
after="$(postgresql_log | grep -c "redo starts" || true)"
[ "$after" = "$before" ] || fail "the start after docker stop recovered the database"

docker kill "$name" >/dev/null
docker start "$name" >/dev/null
healthy "start after docker kill"
recovered="$(postgresql_log | grep -c "redo starts\|database system was interrupted" || true)"
[ "$recovered" -gt 0 ] || fail "the start after docker kill did not recover the database"
echo "image-health-aio: healthy, stopped cleanly, recovered after a kill"
