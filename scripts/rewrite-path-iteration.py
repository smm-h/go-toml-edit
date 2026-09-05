#!/usr/bin/env python3
"""Convert the document's path-based Items/Len calls to the Cursor.

The path-based `(*Document).Items(path)` and `(*Document).Len(path)` are
deleted. The Cursor's own `Items()` and `Len()` answer the same questions from
a position the caller walks to, so a call

    doc.Items("config.tags")        ->  cursorAt(t, doc, "config.tags").Items()
    doc.Len("config.tags")          ->  cursorAt(t, doc, "config.tags").Len()

where `cursorAt` is the test helper that walks a path segment by segment. The
read-layer replacements the design record also names -- `Record.Entries` for
entry enumeration, `Entry.Records` for an array-of-tables' length, the array
node's `Elements` for a plain array's -- are not mechanical and are written by
hand.

Only a call whose path argument is a single double-quoted literal is rewritten;
anything else is reported as MANUAL and left alone.

Usage:
    scripts/rewrite-path-iteration.py --dry-run FILE...
    scripts/rewrite-path-iteration.py --apply FILE...

Exactly one of --dry-run and --apply is required; there is no default. A run
that matches nothing at all exits non-zero: a zero-match run is a mistyped
pattern, not a no-op worth reporting as success.
"""

import argparse
import re
import sys

CALL = re.compile(r"\b(?P<recv>doc|target)\.(?P<method>Items|Len)\((?P<arg>[^()]*)\)")
STRING_ARG = re.compile(r'^"(?:[^"\\]|\\.)*"$')


def rewrite_line(line):
    """Return (new_line, rewritten, manual) for one source line."""
    rewritten = 0
    manual = []

    def sub(m):
        nonlocal rewritten
        arg = m.group("arg").strip()
        if not STRING_ARG.match(arg):
            manual.append(m.group(0))
            return m.group(0)
        rewritten += 1
        return f'cursorAt(t, {m.group("recv")}, {arg}).{m.group("method")}()'

    return CALL.sub(sub, line), rewritten, manual


def main():
    ap = argparse.ArgumentParser()
    mode = ap.add_mutually_exclusive_group(required=True)
    mode.add_argument("--dry-run", action="store_true")
    mode.add_argument("--apply", action="store_true")
    ap.add_argument("files", nargs="+")
    args = ap.parse_args()

    total = 0
    total_manual = 0
    for path in args.files:
        with open(path, encoding="utf-8") as fh:
            lines = fh.readlines()
        out = []
        count = 0
        for number, line in enumerate(lines, 1):
            # A line that only mentions the call in prose is not a call site.
            if line.lstrip().startswith("//"):
                out.append(line)
                continue
            new, rewritten, manual = rewrite_line(line)
            out.append(new)
            count += rewritten
            for text in manual:
                print(f"MANUAL {path}:{number}: {text}")
                total_manual += 1
        if count:
            print(f"{path}: {count}")
            total += count
            if args.apply:
                with open(path, "w", encoding="utf-8") as fh:
                    fh.writelines(out)

    print(f"total: {total} rewritten, {total_manual} left for a human")
    if total == 0:
        print("no call site matched -- check the pattern and the file list",
              file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
