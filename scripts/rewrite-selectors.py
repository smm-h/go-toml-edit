#!/usr/bin/env python3
"""Compiler-driven selector rewrite.

Renaming a struct field or a method leaves every read of it as a type error
naming its exact position. This script asks the compiler where those positions
are and rewrites each one, repeating until the compiler stops reporting them,
so the rewrite is type-aware in a way no textual pattern can be: a field named
`Key` on one type is rewritten while an identically spelled field on another
type is left alone, because only one of them stops compiling.

Usage:

    scripts/rewrite-selectors.py --dry-run (--rule OLD=NEW | --literal OLD=NEW)...
    scripts/rewrite-selectors.py --apply   (--rule OLD=NEW | --literal OLD=NEW)...

`OLD` is the field the compiler reports as unknown, optionally qualified by the
type it was reported on (`StringNode.Val`), which wins over the bare form.
`--rule` replaces the `.OLD` of a selector, so `--rule Value='(Scalar).Value'`
turns `n.Value()` into `n.(Scalar).Value()`. `--literal` replaces the `OLD:` of
a struct-literal field, and a `%` in NEW introduces a template for the value in
which `VALUE` stands for what was written, so
`--literal 'StringNode.Val=val%scalarOf(VALUE)'` turns `&StringNode{Val: "x"}`
into `&StringNode{val: scalarOf("x")}`.

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
    r".*\b(?P<name>\w+) undefined \(type (?P<type>[^\s]+) has no field or method (?P=name)"
)

# `./file.go:12:34: unknown field Name in struct literal of type T`
LITERAL = re.compile(
    r"^(?P<file>[^\s:]+\.go):(?P<line>\d+):(?P<col>\d+): "
    r"unknown field (?P<name>\w+) in struct literal of type (?P<type>[\w.]+)"
)


def value_end(text, start):
    """Return the index just past the value expression beginning at start."""
    depth = 0
    i = start
    while i < len(text):
        c = text[i]
        if c in "([{":
            depth += 1
        elif c in ")]}":
            if depth == 0:
                return i
            depth -= 1
        elif c == "," and depth == 0:
            return i
        elif c in "\"'`":
            quote = c
            i += 1
            while i < len(text) and text[i] != quote:
                if text[i] == "\\" and quote != "`":
                    i += 1
                i += 1
        i += 1
    return len(text)


def find_selector(text, name, start):
    """Index of `.name` at or after start, as a whole selector.

    Without the boundary check `.Val` would match inside `.ValueOf`, which is
    exactly the kind of silent over-match a textual sweep is prone to.
    """
    at = start
    while True:
        at = text.find("." + name, at)
        if at < 0:
            return -1
        after = at + 1 + len(name)
        if after >= len(text) or not (text[after].isalnum() or text[after] == "_"):
            return at
        at = after


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
        line = line.lstrip("./")
        m = ERROR.match(line)
        if m:
            positions.append(
                (m["file"], int(m["line"]), int(m["col"]), m["name"],
                 m["type"].lstrip("*"), "selector")
            )
            continue
        m = LITERAL.match(line)
        if m:
            positions.append(
                (m["file"], int(m["line"]), int(m["col"]), m["name"],
                 m["type"].split(".")[-1], "literal")
            )
            continue
        unmatched.append(line)
    return positions, unmatched


def rule_for(rules, typ, name, kind):
    """The replacement for field name on typ: the type-qualified rule wins."""
    table = rules[kind]
    return table.get(typ + "." + name, table.get(name))


def rewrite(positions, rules, counts):
    """Apply one round of rewrites. Returns the number of edits made."""
    by_file = collections.defaultdict(list)
    for file, line, col, name, typ, kind in positions:
        if rule_for(rules, typ, name, kind) is not None:
            by_file[file].append((line, col, name, typ, kind))

    edits = 0
    for file, sites in by_file.items():
        lines = open(file).read().split("\n")
        # Right to left within a line, so an earlier column stays valid.
        for line, col, name, typ, kind in sorted(sites, reverse=True):
            text = lines[line - 1]
            new = rule_for(rules, typ, name, kind)
            if kind == "selector":
                at = find_selector(text, name, col - 1)
                if at < 0:
                    at = find_selector(text, name, 0)
                if at < 0:
                    print(f"{file}:{line}:{col}: no .{name} on the line",
                          file=sys.stderr)
                    continue
                lines[line - 1] = (
                    text[:at] + "." + new + text[at + 1 + len(name):]
                )
            else:
                at = text.find(name + ":", col - 1)
                if at < 0:
                    at = text.find(name + ":")
                if at < 0:
                    print(f"{file}:{line}:{col}: no {name}: on the line",
                          file=sys.stderr)
                    continue
                field, _, wrap = new.partition("%")
                vstart = at + len(name) + 1
                if wrap:
                    vend = value_end(text, vstart)
                    value = text[vstart:vend].strip()
                    lines[line - 1] = (
                        text[:at] + field + ": " + wrap.replace("VALUE", value)
                        + text[vend:]
                    )
                else:
                    lines[line - 1] = text[:at] + field + text[at + len(name):]
            counts[(file, name)] += 1
            edits += 1
        open(file, "w").write("\n".join(lines))
    return edits


def main():
    ap = argparse.ArgumentParser()
    mode = ap.add_mutually_exclusive_group(required=True)
    mode.add_argument("--dry-run", action="store_true")
    mode.add_argument("--apply", action="store_true")
    ap.add_argument("--rule", action="append", default=[], metavar="OLD=NEW",
                    help="replacement for a selector `.OLD`")
    ap.add_argument("--literal", action="append", default=[],
                    metavar="OLD=NEW", help="replacement for a struct-literal "
                    "field `OLD:`; a %% introduces a value template in which "
                    "VALUE stands for what was written")
    args = ap.parse_args()
    if not args.rule and not args.literal:
        ap.error("at least one --rule or --literal is required")

    rules = {"selector": {}, "literal": {}}
    for kind, specs in (("selector", args.rule), ("literal", args.literal)):
        for spec in specs:
            old, _, new = spec.partition("=")
            if not old or not new:
                ap.error(f"{spec!r} is not OLD=NEW")
            rules[kind][old] = new

    if args.dry_run:
        positions, unmatched = compile_errors()
        counts = collections.Counter()
        for file, _, _, name, typ, kind in positions:
            counts[(file, typ, name, kind)] += 1
        for (file, typ, name, kind), n in sorted(counts.items()):
            mark = "" if rule_for(rules, typ, name, kind) else "   (no rule)"
            print(f"{file}: {n} x {typ}.{name} ({kind}){mark}")
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
            for file, line, col, name, typ, kind in positions:
                print(f"  {file}:{line}:{col}: {typ}.{name} ({kind})",
                      file=sys.stderr)
            return 1
    for (file, name), n in sorted(counts.items()):
        print(f"{file}: {n} x {name}")
    print(f"total: {sum(counts.values())}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
