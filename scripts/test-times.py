#!/usr/bin/env python3
"""Each test's time, from go test -json, with a limit; run by CI's race jobs.

Usage: test-times.py [--limit SECONDS] [--allowlist FILE] [--base REV]
                     [--title TEXT] [--out FOLDER] [--summary FILE]
                     <go test -json file>...

Reads the events go test -json printed (gotestsum's --jsonfile), and:
- writes to the out folder durations.tsv, every test and subtest with its
  time, the slowest first; packages.tsv, each package's time, its tests'
  sum and how many of them ran in parallel; and over-limit.txt, the tests
  over the limit the allowlist lacks, as lines to add to it;
- appends to the summary file (GitHub's job summary) the tests over the
  limit, the slowest tests, and the packages whose tests run one after the
  other;
- with --base, a pull request's base: fails, exiting 1, when a test the
  diff from the base to HEAD adds took longer than the limit, 30 s by
  default, unless the allowlist names it. Any other test over the limit
  gets a warning, as every test does without --base: an existing test near
  the limit, which a busier runner pushes over it, fails no pull request.

A test is a top-level test: a subtest's time counts in its test's. A new
test is a "func TestName(" line the diff adds to a _test.go file, unless it
removes the same test from the same package (a test moved between files).
The allowlist has a line for each test allowed over the limit, "package
Test", the package as go test prints it, then the reason after a #: a new
test slow on purpose, or an existing one whose warning it silences. An
allowlisted test now under the limit is told, so that its line can go.

A package runs its tests serially when, over --serial-seconds (10 s), the
sum of its tests' times is under --serial-below (1.5) times its own: its
tests run about one at a time. A package whose result go test took from its
cache has no time of its own, and is left out.

Python's standard library only, and git for --base.
"""

import argparse
import json
import os
import re
import subprocess
import sys
from collections import defaultdict


class Package:
    def __init__(self):
        self.seconds = None
        self.result = None
        self.cached = False
        # Top-level and subtests: name -> (result, seconds).
        self.tests = {}
        # Top-level tests that called t.Parallel: go test pauses them.
        self.parallel = set()


def read(paths):
    packages = defaultdict(Package)
    for path in paths:
        with open(path, encoding="utf-8", errors="replace") as lines:
            for line in lines:
                # Anything else on the way, such as make's lines, is not an
                # event.
                if not line.startswith("{"):
                    continue
                try:
                    event = json.loads(line)
                except ValueError:
                    continue
                name = event.get("Package")
                if not name:
                    continue
                package = packages[name]
                action = event.get("Action")
                test = event.get("Test")
                if test:
                    if action == "pause" and "/" not in test:
                        package.parallel.add(test)
                    elif action in ("pass", "fail", "skip") and "Elapsed" in event:
                        package.tests[test] = (action, float(event["Elapsed"]))
                elif action == "output" and "(cached)" in event.get("Output", ""):
                    package.cached = True
                elif action in ("pass", "fail", "skip") and "Elapsed" in event:
                    package.result = action
                    package.seconds = float(event["Elapsed"])
    return packages


def read_allowlist(path):
    allowed = {}
    if not path:
        return allowed
    with open(path, encoding="utf-8") as lines:
        for number, line in enumerate(lines, 1):
            entry, _, reason = line.partition("#")
            words = entry.split()
            if not words:
                continue
            if len(words) != 2 or not words[1].startswith("Test") or "/" in words[1]:
                sys.exit(f'{path}:{number}: expected "package TestName # reason", got {line.strip()!r}')
            allowed[(words[0], words[1])] = reason.strip()
    return allowed


def new_tests(base):
    """The top-level tests the diff from base to HEAD adds, as (package, Test)."""
    root = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"], check=True, capture_output=True, text=True
    ).stdout.strip()
    with open(os.path.join(root, "go.mod"), encoding="utf-8") as lines:
        module = next(line.split()[1] for line in lines if line.startswith("module "))
    diff = subprocess.run(
        ["git", "diff", "--unified=0", "--no-color", "--no-ext-diff", base, "HEAD", "--", "*_test.go"],
        check=True,
        capture_output=True,
        text=True,
        cwd=root,
    ).stdout
    added, removed = set(), set()
    old = new = None
    function = re.compile(r"func (Test\w*)\(")
    for line in diff.splitlines():
        if line.startswith("--- "):
            old = line[6:] if line.startswith("--- a/") else None
        elif line.startswith("+++ "):
            new = line[6:] if line.startswith("+++ b/") else None
        elif line.startswith("+") and new and (found := function.match(line, 1)):
            added.add((package_of(module, new), found.group(1)))
        elif line.startswith("-") and old and (found := function.match(line, 1)):
            removed.add((package_of(module, old), found.group(1)))
    return added - removed


def package_of(module, path):
    folder = os.path.dirname(path)
    return f"{module}/{folder}" if folder else module


def seconds(value):
    return f"{value:.1f}"


def plural(count, word):
    return word if count == 1 else word + "s"


def table(head, rows):
    lines = ["| " + " | ".join(head) + " |", "|" + "---|" * len(head)]
    lines += ["| " + " | ".join(str(cell) for cell in row) + " |" for row in rows]
    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("files", nargs="+", help="go test -json output")
    parser.add_argument("--limit", type=float, default=30, help="seconds a test may take (30)")
    parser.add_argument("--allowlist", help="tests allowed over the limit")
    parser.add_argument("--base", help="a pull request's base: only the tests the diff from it adds may fail")
    parser.add_argument("--title", default="Go tests", help="the summary's heading")
    parser.add_argument("--out", help="folder for durations.tsv, packages.tsv and over-limit.txt")
    parser.add_argument("--summary", help="Markdown file to append the summary to")
    parser.add_argument("--slowest", type=int, default=15, help="slowest tests the summary lists (15)")
    parser.add_argument("--serial-seconds", type=float, default=10, help="time from which a package is judged (10)")
    parser.add_argument(
        "--serial-below", type=float, default=1.5, help="tests' sum over package time under which it is serial (1.5)"
    )
    args = parser.parse_args()

    packages = read(args.files)
    allowed = read_allowlist(args.allowlist)

    tests = []  # (seconds, result, package, test), top-level only
    every = []  # the same with subtests
    for name, package in packages.items():
        for test, (result, elapsed) in package.tests.items():
            every.append((elapsed, result, name, test))
            if "/" not in test:
                tests.append((elapsed, result, name, test))
    tests.sort(key=lambda row: (-row[0], row[2], row[3]))
    every.sort(key=lambda row: (-row[0], row[2], row[3]))

    over = [row for row in tests if row[0] > args.limit and row[1] != "skip"]
    unlisted = [row for row in over if (row[2], row[3]) not in allowed]
    listed = [row for row in over if (row[2], row[3]) in allowed]
    # Only a pull request's new tests fail; the others warn.
    added = new_tests(args.base) if args.base else set()
    failing = [row for row in unlisted if (row[2], row[3]) in added]
    warned = [row for row in unlisted if (row[2], row[3]) not in added]
    ran = {(row[2], row[3]): row for row in tests}
    under = [ran[key] for key in allowed if key in ran and ran[key][0] <= args.limit]

    rows = []  # packages.tsv
    serial = []
    for name, package in sorted(packages.items()):
        top = [package.tests[t][1] for t in package.tests if "/" not in t]
        if package.seconds is None:
            continue
        total = sum(top)
        concurrency = total / package.seconds if package.seconds > 0 else 0
        rows.append((name, package.seconds, len(top), len(package.parallel), total, concurrency, package.cached))
        if (
            not package.cached
            and package.seconds >= args.serial_seconds
            and len(top) > 1
            and concurrency < args.serial_below
        ):
            serial.append(rows[-1])
    serial.sort(key=lambda row: -row[1])

    if args.out:
        os.makedirs(args.out, exist_ok=True)
        with open(os.path.join(args.out, "durations.tsv"), "w", encoding="utf-8") as out:
            out.write("seconds\tresult\tpackage\ttest\n")
            for elapsed, result, name, test in every:
                out.write(f"{seconds(elapsed)}\t{result}\t{name}\t{test}\n")
        with open(os.path.join(args.out, "packages.tsv"), "w", encoding="utf-8") as out:
            out.write("package\tseconds\ttests\tparallel\ttests_seconds\tconcurrency\tcached\n")
            for name, elapsed, count, parallel, total, concurrency, cached in rows:
                out.write(
                    f"{name}\t{seconds(elapsed)}\t{count}\t{parallel}\t{seconds(total)}\t{concurrency:.2f}\t{str(cached).lower()}\n"
                )
        with open(os.path.join(args.out, "over-limit.txt"), "w", encoding="utf-8") as out:
            for elapsed, result, name, test in unlisted:
                out.write(f"{name} {test} # {seconds(elapsed)} s in {args.title}\n")

    limit = f"{args.limit:g} s"
    allowlist = args.allowlist or "the allowlist"
    tested = sum(1 for p in packages.values() if p.tests)
    counted = f"{len(tests)} {plural(len(tests), 'test')} in {tested} {plural(tested, 'package')}"
    scope = f"{len(added)} new against {args.base}" if args.base else "no base, so none fails"
    print(f"{counted} ({scope}); {len(over)} over {limit}, {len(listed)} of them allowlisted")
    for elapsed, result, name, test in failing:
        print(
            f"::error title=New test over {limit}::{name} {test}, new in this pull request, took {seconds(elapsed)} s, "
            f'over {limit}: make it faster, or add "{name} {test} # why" to {allowlist}'
        )
    why = "not new in this pull request" if args.base else "no pull request's base to tell new tests by"
    for elapsed, result, name, test in warned:
        print(
            f"::warning title=Test over {limit}::{name} {test} took {seconds(elapsed)} s, over {limit}: "
            f"{why}, so a warning only"
        )
    for elapsed, result, name, test in under:
        print(f"Allowlisted but under {limit}: {name} {test} took {seconds(elapsed)} s")
    for name, elapsed, count, parallel, total, _, _ in serial:
        print(
            f"Serial: {name} took {seconds(elapsed)} s for {seconds(total)} s of tests ({count} tests, {parallel} in parallel)"
        )

    if args.summary:
        parts = [f"### {args.title}: test times", ""]
        parts.append(
            f"{counted} ({scope}). Over {limit}: {len(over)}, of which {len(listed)} on the allowlist, "
            f"{len(failing)} new and failing, {len(warned)} warned."
        )
        if failing:
            parts += [
                "",
                (
                    f"**New tests over {limit}, not on the allowlist** (the job fails): make them faster, "
                    f"or add these lines to `{allowlist}` with the reason after the #."
                ),
                "",
                "```",
                *[f"{name} {test} # {seconds(elapsed)} s in {args.title}" for elapsed, _, name, test in failing],
                "```",
            ]
        if warned:
            parts += ["", f"**Tests over {limit}, warned** ({why}):", ""]
            parts.append(table(["Seconds", "Package", "Test"], [(seconds(e), n, t) for e, _, n, t in warned]))
        if listed:
            parts += ["", f"**Allowlisted, over {limit}:**", ""]
            parts.append(
                table(
                    ["Seconds", "Package", "Test", "Reason"],
                    [(seconds(e), n, t, allowed[(n, t)]) for e, _, n, t in listed],
                )
            )
        if under:
            parts += ["", f"**Allowlisted, now under {limit}** (their lines can go):", ""]
            parts.append(table(["Seconds", "Package", "Test"], [(seconds(e), n, t) for e, _, n, t in under]))
        parts += ["", f"**The {args.slowest} slowest tests:**", ""]
        parts.append(
            table(
                ["Seconds", "Result", "Package", "Test"],
                [(seconds(e), r, n, t) for e, r, n, t in tests[: args.slowest]],
            )
        )
        parts += [
            "",
            (
                f"**Packages running their tests serially** (over {args.serial_seconds:g} s, tests' sum under "
                f"{args.serial_below:g} times the package's time):"
            ),
            "",
        ]
        if serial:
            parts.append(
                table(
                    ["Package", "Seconds", "Tests", "In parallel", "Tests' sum", "Sum / time"],
                    [(n, seconds(e), c, p, seconds(t), f"{x:.2f}") for n, e, c, p, t, x, _ in serial],
                )
            )
        else:
            parts.append("None.")
        parts.append("")
        with open(args.summary, "a", encoding="utf-8") as out:
            out.write("\n".join(parts) + "\n")

    return 1 if failing else 0


if __name__ == "__main__":
    sys.exit(main())
