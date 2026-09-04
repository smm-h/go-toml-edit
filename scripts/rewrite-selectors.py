#!/usr/bin/env python3
"""Compiler-driven selector rewrite.

Renaming a struct field or a method leaves every read of it as a type error
naming its exact position. This script asks the compiler where those positions
are and rewrites each one, repeating until the compiler stops reporting them,
so the rewrite is type-aware in a way no textual pattern can be: a field named
`Key` on one type is rewritten while an identically spelled field on another
type is left alone, because only one of them stops compiling.

Usage:

    scripts/rewrite-selectors.py --dry-run  --rule OLD=NEW [--rule ...]
    scripts/rewrite-selectors.py --apply    --rule OLD=NEW [--rule ...]

`OLD` is the selector name the compiler reports as undefined; `NEW` is what
replaces the `.OLD` text at that position, so `--rule Value='(Scalar).Value'`
turns `n.Value()` into `n.(Scalar).Value()` and `--rule Children=children`
turns `d.Children` into `d.children`.

Exactly one of --dry-run and --apply is required; there is no default. The dry
run reports the positions the first compile round finds and writes nothing --
it cannot see the rounds after it, because those positions only become visible
once the earlier ones compile.

Exit status is non-zero when the compiler still reports errors this script
cannot attribute to one of its rules.
"""

import argparse
import collections
import re
import subprocess
import sys

# `./file.go:12:34: expr.Name undefined (type T has no field or method Name)`
ERROR = re.compile(
    r"^(?P<file>[^\s:]+\.go):(?P<line>\d+):(?P<col>\d+): "
    r".*\b(?P<name>\w+) undefined \(type .* has no field or method (?P=name)"
)


def compile_errors():
    """Return (positions, unmatched) from one type-check of the package."""
    proc = subprocess.run(
        ["go", "vet", "./..."], capture_output=True, text=True
    )
    positions, unmatched = [], []
    for raw in proc.stderr.splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "too many errors" in line:
            continue
        # `go vet` prefixes the first error of a package's external test
        # variant with "vet: ", and paths with "./".
        if line.startswith("vet: "):
            line = line[len("vet: "):]
        m = ERROR.match(line.lstrip("./"))
        if m:
            positions.append(
                (m["file"], int(m["line"]), int(m["col"]), m["name"])
            )
        else:
            unmatched.append(line)
    return positions, unmatched


def rewrite(positions, rules, counts):
    """Apply one round of rewrites. Returns the number of edits made."""
    by_file = collections.defaultdict(list)
    for file, line, col, name in positions:
        if name in rules:
            by_file[file].append((line, col, name))

    edits = 0
    for file, sites in by_file.items():
        lines = open(file).read().split("\n")
        # Right to left within a line, so an earlier column stays valid.
        for line, col, name in sorted(sites, reverse=True):
            text = lines[line - 1]
            at = text.find("." + name, col - 1)
            if at < 0:
                at = text.find("." + name)
            if at < 0:
                print(f"{file}:{line}:{col}: no .{name} on the line", file=sys.stderr)
                continue
            lines[line - 1] = (
                text[:at] + "." + rules[name] + text[at + 1 + len(name):]
            )
            counts[(file, name)] += 1
            edits += 1
        open(file, "w").write("\n".join(lines))
    return edits


def main():
    ap = argparse.ArgumentParser()
    mode = ap.add_mutually_exclusive_group(required=True)
    mode.add_argument("--dry-run", action="store_true")
    mode.add_argument("--apply", action="store_true")
    ap.add_argument("--rule", action="append", required=True, metavar="OLD=NEW")
    args = ap.parse_args()

    rules = {}
    for spec in args.rule:
        old, _, new = spec.partition("=")
        if not old or not new:
            ap.error(f"--rule {spec!r} is not OLD=NEW")
        rules[old] = new

    if args.dry_run:
        positions, unmatched = compile_errors()
        counts = collections.Counter()
        for file, _, _, name in positions:
            counts[(file, name)] += 1
        for (file, name), n in sorted(counts.items()):
            mark = "" if name in rules else "   (no rule)"
            print(f"{file}: {n} x .{name}{mark}")
        for line in unmatched:
            print(f"unmatched: {line}")
        print(f"total this round: {sum(counts.values())}")
        return 0

    counts = collections.Counter()
    for round_no in range(1, 200):
        positions, unmatched = compile_errors()
        if not positions:
            if unmatched:
                print("remaining compiler output:", file=sys.stderr)
                for line in unmatched:
                    print("  " + line, file=sys.stderr)
                return 1
            break
        if rewrite(positions, rules, counts) == 0:
            print("no rule matched the reported positions:", file=sys.stderr)
            for file, line, col, name in positions:
                print(f"  {file}:{line}:{col}: .{name}", file=sys.stderr)
            return 1
    for (file, name), n in sorted(counts.items()):
        print(f"{file}: {n} x .{name} -> .{rules[name]}")
    print(f"total: {sum(counts.values())}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
