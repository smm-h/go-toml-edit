# Sync backfilled release descriptions to the old GitHub Releases

## Context

The release ledger's archives for v0.0.1, v0.1.0, v0.1.1, and v0.1.2 were
backfilled (anchors, format_version, and authored descriptions recovered
from those versions' changelog entries). The GitHub Releases for those
tags — where they exist — still carry whatever notes they originally had,
possibly none. The archives are the authoritative ledger; the GitHub
Release body is a projection of it.

## Problem

The projections for the four earliest versions do not reflect the authored
descriptions. Cosmetic only: no tooling reads those old Release bodies.

## Solution

Run the release-notes re-sync (`rlsbl release edit <version>`) for each of
the four versions, which regenerates the GitHub Release notes from the
changelog section and archived metadata. Verify each Release body
afterwards.

## Affected surface

External only: the four GitHub Release bodies. No repository files change.

## Effort

Minutes.
