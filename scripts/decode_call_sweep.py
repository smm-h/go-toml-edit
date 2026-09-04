#!/usr/bin/env python3
"""Convert pointer-taking decode calls into the value-returning generic forms.

The decode entry points changed shape:

    var cfg Config                              cfg, err := Unmarshal[Config](data)
    err := Unmarshal(data, &cfg)          ->    ...
    err := doc.Decode(&cfg)                     cfg, err := Decode[Config](doc)
    err := DecodeNode(node, &cfg)               cfg, err := DecodeNode[Config](node)

This rewrites the mechanical majority: a call whose target variable is
declared by a plain `var name Type` line within the preceding few lines. The
declaration is deleted and the call rebound. Everything else is reported as
MANUAL and left alone -- it needs a human decision (a target built by a
closure, a repeated target, a target the test deliberately mis-shapes).

Usage:
    scripts/decode_call_sweep.py --dry-run FILE...
    scripts/decode_call_sweep.py --apply FILE...
"""

import argparse
import re
import sys

CALL = re.compile(
    r"^(?P<indent>\s*)"
    r"(?:if )?"
    r"(?P<lhs>err)\s*(?P<op>:?=)\s*"
    r"(?P<pkg>[A-Za-z_][A-Za-z0-9_]*\.)?"
    r"(?P<fn>Unmarshal|DecodeNode)\((?P<args>.*), &(?P<var>[A-Za-z_][A-Za-z0-9_]*)\)"
    r"(?P<tail>; err != nil \{|)\s*$"
)

METHOD = re.compile(
    r"^(?P<indent>\s*)"
    r"(?:if )?"
    r"(?P<lhs>err)\s*(?P<op>:?=)\s*"
    r"(?P<recv>[A-Za-z_][A-Za-z0-9_]*)\.Decode\(&(?P<var>[A-Za-z_][A-Za-z0-9_]*)\)"
    r"(?P<tail>; err != nil \{|)\s*$"
)

DECL = re.compile(r"^\s*var (?P<var>[A-Za-z_][A-Za-z0-9_]*) (?P<type>[^=]+)$")

LOOKBACK = 12


def find_decl(lines, index, name):
    """Return the index of `var name Type` above index, or None."""
    for i in range(index - 1, max(-1, index - 1 - LOOKBACK), -1):
        m = DECL.match(lines[i])
        if m and m.group("var") == name:
            return i
    return None


def convert(path, apply):
    with open(path) as fh:
        lines = fh.read().split("\n")

    out = list(lines)
    drop = set()
    converted = 0
    manual = []

    for i, line in enumerate(lines):
        m = CALL.match(line)
        kind = "func"
        if not m:
            m = METHOD.match(line)
            kind = "method"
        if not m:
            continue
        if m.group("pkg") if kind == "func" else False:
            manual.append((i + 1, line.strip(), "package-qualified"))
            continue
        name = m.group("var")
        decl = find_decl(lines, i, name)
        if decl is None or decl in drop:
            manual.append((i + 1, line.strip(), "no plain var declaration above"))
            continue
        typ = DECL.match(lines[decl]).group("type").strip()
        indent = m.group("indent")
        if kind == "func":
            call = f"{m.group('fn')}[{typ}]({m.group('args')})"
        else:
            call = f"Decode[{typ}]({m.group('recv')})"
        new = f"{indent}{name}, err := {call}"
        if m.group("tail"):
            new += f"\n{indent}if err != nil {{"
        out[i] = new
        drop.add(decl)
        converted += 1

    for i in drop:
        out[i] = None
    text = "\n".join(l for l in out if l is not None)

    if apply and converted:
        with open(path, "w") as fh:
            fh.write(text)
    return converted, manual


def main():
    ap = argparse.ArgumentParser()
    mode = ap.add_mutually_exclusive_group(required=True)
    mode.add_argument("--dry-run", action="store_true")
    mode.add_argument("--apply", action="store_true")
    ap.add_argument("files", nargs="+")
    args = ap.parse_args()

    total = 0
    manual_total = 0
    for path in args.files:
        converted, manual = convert(path, args.apply)
        total += converted
        manual_total += len(manual)
        if converted or manual:
            print(f"{path}: {converted} converted, {len(manual)} manual")
        for line, text, why in manual:
            print(f"  MANUAL {path}:{line}: {why}: {text}")
    print(f"total: {total} converted, {manual_total} manual")
    return 0


if __name__ == "__main__":
    sys.exit(main())
