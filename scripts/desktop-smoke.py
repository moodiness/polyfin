#!/usr/bin/env python3
"""Smoke test of Polyfin Desktop, run by CI on each system.

Usage: desktop-smoke.py <polyfin-desktop executable> <install folder> <work folder>

The install folder holds polyfin (polyfin.exe) and postgresql/{bin,lib,share},
as the packages lay them out. The app runs with -headless, on a data folder
of the work folder, twice: the first start creates the cluster, downloads a
fake FFmpeg from a local server and starts the server, whose admin app must
answer; the app is then interrupted, and none of its processes may remain.
The second start must reuse the cluster. Every request for outside goes
through a local proxy that refuses it, and is listed.

Python's standard library only: it runs on Linux, macOS and Windows runners.
"""
import ctypes
import hashlib
import http.server
import json
import os
import re
import signal
import socket
import subprocess
import sys
import threading
import time
import urllib.request
import zipfile

WINDOWS = os.name == "nt"


# DESKTOP_SMOKE_PORTS, such as 19000-19009, keeps the ports in a range on
# a shared machine; else free ports are taken.
_given = set()


def free_port():
    first, _, last = os.environ.get("DESKTOP_SMOKE_PORTS", "0-0").partition("-")
    for port in range(int(first), int(last or first) + 1):
        if port in _given:
            continue
        with socket.socket() as s:
            try:
                s.bind(("127.0.0.1", port))
            except OSError:
                continue
            port = s.getsockname()[1]
        _given.add(port)
        return port
    sys.exit("no free port in DESKTOP_SMOKE_PORTS")


class Fake(http.server.BaseHTTPRequestHandler):
    """Serves the fake tools, and refuses every request for outside."""

    tools = b""
    refused = []

    def log_message(self, fmt, *args):
        sys.stderr.write("fake: " + (fmt % args) + "\n")

    def do_CONNECT(self):
        Fake.refused.append(self.path)
        self.send_response(403)
        self.end_headers()

    def do_GET(self):
        if self.path.startswith(("http://", "https://")):
            Fake.refused.append(self.path)
            self.send_response(403)
            self.end_headers()
            return
        if self.path == "/tools.zip":
            self.send_response(200)
            self.send_header("Content-Length", str(len(Fake.tools)))
            self.end_headers()
            self.wfile.write(Fake.tools)
            return
        self.send_response(404)
        self.end_headers()

    do_POST = do_GET


def alive(pid):
    if WINDOWS:
        kernel32 = ctypes.windll.kernel32
        handle = kernel32.OpenProcess(0x1000, False, pid)  # PROCESS_QUERY_LIMITED_INFORMATION
        if not handle:
            return False
        code = ctypes.c_ulong()
        kernel32.GetExitCodeProcess(handle, ctypes.byref(code))
        kernel32.CloseHandle(handle)
        return code.value == 259  # STILL_ACTIVE
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    # A zombie is not running; its parent, the app, has ended.
    try:
        state = subprocess.run(["ps", "-o", "stat=", "-p", str(pid)], capture_output=True, text=True).stdout.strip()
        return state != "" and not state.startswith("Z")
    except OSError:
        return True


def processes_of(folder):
    """The processes whose command line names folder, but for this script
    and the shell that started it."""
    if WINDOWS:
        script = ("Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -like '*%s*' } "
                  "| ForEach-Object { \"$($_.ProcessId) $($_.CommandLine)\" }") % folder.replace("'", "''")
        out = subprocess.run(["powershell", "-NoProfile", "-Command", script], capture_output=True, text=True).stdout
    else:
        out = subprocess.run(["ps", "-axo", "pid=,command="], capture_output=True, text=True).stdout
        out = "\n".join(line for line in out.splitlines() if folder in line)
    mine = {str(os.getpid()), str(os.getppid())}
    return [line.strip() for line in out.splitlines()
            if line.strip() and line.split()[0] not in mine and "Get-CimInstance" not in line]


def wait_ready(listen, timeout):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with urllib.request.urlopen("http://%s/admin/api/status" % listen, timeout=5) as r:
                status = json.load(r)
                if status.get("database") == "ready":
                    return status
        except (OSError, ValueError):
            pass
        time.sleep(1)
    return None


def run_app(app, env, data, install, listen):
    """Starts the app, waits for the admin app, interrupts the app, and
    returns the PIDs it ran."""
    flags = subprocess.CREATE_NEW_PROCESS_GROUP if WINDOWS else 0
    process = subprocess.Popen([app, "-headless"], env=env, creationflags=flags)
    status = wait_ready(listen, 300)
    log = open(os.path.join(data, "log", "desktop.log"), encoding="utf-8").read()
    if status is None:
        process.kill()
        sys.exit("the admin app never answered with its database ready:\n" + log)
    print("status:", json.dumps(status))
    servers = [int(p) for p in re.findall(r'msg="The server started" pid=(\d+)', log)]
    with open(os.path.join(data, "postgresql", "postmaster.pid")) as f:
        postmaster = int(f.readline())
    pids = {"app": process.pid, "server": servers[-1], "postmaster": postmaster}
    print("processes:", pids)
    if WINDOWS:
        process.send_signal(signal.CTRL_BREAK_EVENT)
    else:
        process.send_signal(signal.SIGTERM)
    try:
        code = process.wait(timeout=240)
    except subprocess.TimeoutExpired:
        process.kill()
        sys.exit("the app did not quit within 4 minutes")
    print("the app quit with", code)
    time.sleep(2)
    left = {name: pid for name, pid in pids.items() if alive(pid)}
    if left:
        sys.exit("processes left after quitting: %s" % left)
    if os.path.exists(os.path.join(data, "postgresql", "postmaster.pid")):
        sys.exit("the cluster's lock file is left")
    remaining = processes_of(data) + processes_of(install)
    if remaining:
        sys.exit("processes of the data or install folder left:\n" + "\n".join(remaining))
    return status, log


def main():
    app, install, work = (os.path.abspath(a) for a in sys.argv[1:4])
    data = os.path.join(work, "data")
    os.makedirs(work, exist_ok=True)

    # A fake FFmpeg, laid out as the macOS downloads: one archive, into bin.
    suffix = ".exe" if WINDOWS else ""
    archive = os.path.join(work, "tools.zip")
    with zipfile.ZipFile(archive, "w") as z:
        for name in ("ffmpeg", "ffprobe"):
            z.writestr(name + suffix, "fake " + name + "\n")
    Fake.tools = open(archive, "rb").read()
    fake_port = free_port()
    server = http.server.ThreadingHTTPServer(("127.0.0.1", fake_port), Fake)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    manifest = os.path.join(work, "tools.json")
    with open(manifest, "w") as f:
        json.dump({"version": "9.0.2", "ffmpeg": "bin/ffmpeg" + suffix, "ffprobe": "bin/ffprobe" + suffix,
                   "files": [{"url": "http://127.0.0.1:%d/tools.zip" % fake_port, "format": "zip", "into": "bin",
                              "sha256": hashlib.sha256(Fake.tools).hexdigest(), "size": len(Fake.tools)}]}, f)

    listen = "127.0.0.1:%d" % free_port()
    proxy = "http://127.0.0.1:%d" % fake_port
    env = dict(os.environ)
    env.update({
        "POLYFIN_DESKTOP_INSTALL_DIR": install,
        "POLYFIN_DESKTOP_DATA_DIR": data,
        "POLYFIN_DESKTOP_TOOLS": manifest,
        "POLYFIN_DESKTOP_DATABASE_PORT": str(free_port()),
        "POLYFIN_LISTEN": listen,
        "POLYFIN_DISCOVERY_PORT": str(free_port()),
        "POLYFIN_DLNA_PORT": str(free_port()),
        "POLYFIN_HDHOMERUN_PORT": str(free_port()),
        "HTTP_PROXY": proxy, "HTTPS_PROXY": proxy, "http_proxy": proxy, "https_proxy": proxy,
        "NO_PROXY": "127.0.0.1,localhost", "no_proxy": "127.0.0.1,localhost",
    })

    # The check after quitting counts every process of the two folders:
    # nothing else may run from them, such as a test cluster started from
    # the same install folder.
    running = processes_of(data) + processes_of(install)
    if running:
        sys.exit("processes of the data or install folder run before the smoke starts:\n" + "\n".join(running))

    print("== first start")
    status, log = run_app(app, env, data, install, listen)
    if not status.get("setupRequired"):
        sys.exit("the first start has an administrator already")
    if "Creating the database cluster" not in log or "FFmpeg and ffprobe are ready" not in log:
        sys.exit("the first start did not create the cluster or get the tools:\n" + log)
    if "secret key could not be read" in log:
        print("note: the runner has no key store; the server ran without its key")
    first = log

    print("== second start")
    _, log = run_app(app, env, data, install, listen)
    second = log[len(first):]
    if "Creating the database cluster" in second or "Downloading FFmpeg" in second:
        sys.exit("the second start did not reuse the cluster and the tools:\n" + second)
    if "A secret key was made" in second:
        sys.exit("the second start made another key:\n" + second)
    print("refused outside requests:", sorted(set(Fake.refused)) or "none")
    print("ok")


if __name__ == "__main__":
    main()
