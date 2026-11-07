#!/usr/bin/env python3
"""Each test's time, from go test -json, with a limit; run by CI's race jobs.

Usage: test-times.py [--limit SECONDS] [--allowlist FILE] [--title TEXT]
                     [--out FOLDER] [--summary FILE] <go test -json file>...

Reads the events go test -json printed (gotestsum's --jsonfile), and:
- writes to the out folder durations.tsv, every test and subtest with its
  time, the slowest first; packages.tsv, each package's time, its tests'
  sum and how many of them ran in parallel; and over-limit.txt, the tests
  over the limit the allowlist lacks, as lines to add to it;
- appends to the summary file (GitHub's job summary) the tests over the
  limit, the slowest tests, and the packages whose tests run one after the
  other;
- fails, exiting 1, when a test took longer than the limit, 30 s by
  default, unless the allowlist names it.

A test is a top-level test: a subtest's time counts in its test's. The
allowlist has a line for each test allowed over the limit, "package Test",
the package as go test prints it, then the reason after a #. An allowlisted
test now under the limit is told, so that its line can go.

A package runs its tests serially when, over --serial-seconds (10 s), the
sum of its tests' times is under --serial-below (1.5) times its own: its
tests run about one at a time. A package whose result go test took from its
cache has no time of its own, and is left out.

Python's standard library only.
"""

import argparse
import json
import os
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
    print(f"{counted}; {len(over)} over {limit}, {len(listed)} of them allowlisted")
    for elapsed, result, name, test in unlisted:
        print(
            f"::error title=Test over {limit}::{name} {test} took {seconds(elapsed)} s, over {limit}: "
            f'make it faster, or add "{name} {test} # why" to {allowlist}'
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
            f"{counted}. Over {limit}: {len(over)}, of which {len(listed)} on the allowlist and {len(unlisted)} not."
        )
        if unlisted:
            parts += [
                "",
                (
                    f"**Over {limit} and not on the allowlist** (the job fails): make them faster, "
                    f"or add these lines to `{allowlist}` with the reason after the #."
                ),
                "",
                "```",
                *[f"{name} {test} # {seconds(elapsed)} s in {args.title}" for elapsed, _, name, test in unlisted],
                "```",
            ]
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

    return 1 if unlisted else 0


if __name__ == "__main__":
    sys.exit(main())
