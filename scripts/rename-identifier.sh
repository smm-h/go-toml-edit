#!/usr/bin/env bash
#
# Identifier-boundary-aware rename sweep.
#
# Rewrites whole identifiers across the given files. A name is rewritten only
# where it is not flanked by [A-Za-z0-9_], so `Rename` never matches inside
# `RenameKey`, `renameKeyInParent` or `TestRename_RoundTrip`, and
# `DocumentNode` never matches inside `ExampleDocumentNode_Set`. There is no
# substring mode: superstring identifiers must be named as their own pairs.
#
# Usage:
#   scripts/rename-identifier.sh (--dry-run | --apply) \
#       --pair OLD=NEW [--pair OLD=NEW ...] \
#       [--exclude-line FILE:LINE ...] \
#       -- FILE [FILE ...]
#
#   --dry-run   report per-file, per-pair occurrence counts; write nothing.
#   --apply     perform the rewrite and report what it wrote.
#               Exactly one of the two is required; there is no default.
#   --pair      OLD=NEW, repeatable. Pairs are applied in the order given.
#   --exclude-line
#               pin one FILE:LINE out of the sweep, for an occurrence that is
#               the English word rather than the identifier. Repeatable.
#               Excluded occurrences are counted and reported separately.
#
# A sweep that matches nothing at all exits non-zero: a zero-match run is a
# mistyped pattern, not a no-op worth reporting as success.

set -euo pipefail

mode=""
pairs=()
excludes=()
files=()

die() {
	printf 'rename-identifier: %s\n' "$1" >&2
	exit 2
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--dry-run)
		[[ -n "$mode" ]] && die "--dry-run and --apply are mutually exclusive"
		mode="dry-run"
		shift
		;;
	--apply)
		[[ -n "$mode" ]] && die "--dry-run and --apply are mutually exclusive"
		mode="apply"
		shift
		;;
	--pair)
		[[ $# -ge 2 ]] || die "--pair needs an OLD=NEW argument"
		[[ "$2" == *=* ]] || die "--pair argument must be OLD=NEW, got: $2"
		pairs+=("$2")
		shift 2
		;;
	--exclude-line)
		[[ $# -ge 2 ]] || die "--exclude-line needs a FILE:LINE argument"
		[[ "$2" == *:* ]] || die "--exclude-line argument must be FILE:LINE, got: $2"
		excludes+=("$2")
		shift 2
		;;
	--)
		shift
		files=("$@")
		break
		;;
	*)
		die "unknown argument: $1"
		;;
	esac
done

[[ -n "$mode" ]] || die "one of --dry-run or --apply is required"
[[ ${#pairs[@]} -gt 0 ]] || die "at least one --pair is required"
[[ ${#files[@]} -gt 0 ]] || die "at least one file is required after --"

for f in "${files[@]}"; do
	[[ -f "$f" ]] || die "not a file: $f"
done

# The perl half: boundary-aware count-and-substitute for one pair in one file.
# Parameters arrive through the environment so no shell quoting reaches perl.
perl_prog='
my ($old, $new, $file, $skip, $mode) =
    @ENV{qw(RI_OLD RI_NEW RI_FILE RI_SKIP RI_MODE)};
my %skip = map { $_ => 1 } grep { length } split /,/, $skip;
open my $in, "<", $file or die "cannot read $file: $!\n";
my @lines = <$in>;
close $in;
my ($count, $skipped) = (0, 0);
for my $i (0 .. $#lines) {
    if ($skip{$i + 1}) {
        $skipped += () = $lines[$i] =~ /(?<![A-Za-z0-9_])\Q$old\E(?![A-Za-z0-9_])/g;
        next;
    }
    $count += ($lines[$i] =~ s/(?<![A-Za-z0-9_])\Q$old\E(?![A-Za-z0-9_])/$new/g);
}
if ($mode eq "apply" && $count > 0) {
    open my $out, ">", $file or die "cannot write $file: $!\n";
    print $out @lines;
    close $out;
}
print "$count $skipped\n";
'

printf 'mode: %s\n\n' "$mode"

total=0
total_skipped=0

for pair in "${pairs[@]}"; do
	old="${pair%%=*}"
	new="${pair#*=}"
	[[ -n "$old" && -n "$new" ]] || die "malformed pair: $pair"
	printf '%s -> %s\n' "$old" "$new"
	pair_total=0
	for f in "${files[@]}"; do
		skip=""
		for ex in "${excludes[@]:-}"; do
			[[ -z "$ex" ]] && continue
			exfile="${ex%:*}"
			exline="${ex##*:}"
			if [[ "$exfile" == "$f" ]]; then
				skip="${skip}${exline},"
			fi
		done
		read -r count skipped < <(
			RI_OLD="$old" RI_NEW="$new" RI_FILE="$f" RI_SKIP="$skip" RI_MODE="$mode" \
				perl -e "$perl_prog"
		)
		if [[ "$count" -gt 0 || "$skipped" -gt 0 ]]; then
			printf '  %-32s %4d' "$f" "$count"
			[[ "$skipped" -gt 0 ]] && printf '  (%d excluded by --exclude-line)' "$skipped"
			printf '\n'
		fi
		pair_total=$((pair_total + count))
		total=$((total + count))
		total_skipped=$((total_skipped + skipped))
	done
	printf '  %-32s %4d\n\n' "TOTAL" "$pair_total"
done

printf 'all pairs: %d occurrences, %d excluded by --exclude-line\n' "$total" "$total_skipped"

if [[ "$total" -eq 0 ]]; then
	die "no occurrences matched -- check the pairs and the file list"
fi
