#!/usr/bin/env bash
# build-image.sh — build the Commentarr container image.
#
# Until the commentary-classifier module is published to a public registry,
# this repo's go.mod uses a `replace` directive pointing at a sibling
# checkout. A raw `podman build .` fails because the sibling dir isn't
# inside the build context.
#
# This script assembles a self-contained build context in a temp dir with
# both commentarr/ and commentary-classifier/ as siblings, then runs the
# build from there. Once the classifier ships a tagged release we swap
# the replace for a version pin and can drop this script.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SIBLING="$(cd "$REPO_ROOT/../commentary-classifier" && pwd)"
TAG="${IMAGE_TAG:-commentarr:plan1-dev}"

echo "commentarr repo:       $REPO_ROOT"
echo "classifier sibling:    $SIBLING"
echo "image tag:             $TAG"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# rsync preserves permissions and excludes VCS / build artifacts
rsync -a --exclude '.git' --exclude 'vendor' --exclude 'bin' \
      --exclude '*.test' --exclude '/commentarr' \
      "$REPO_ROOT/" "$TMP/commentarr/"
rsync -a --exclude '.git' --exclude 'vendor' --exclude 'bin' \
      --exclude '*.test' \
      "$SIBLING/" "$TMP/commentary-classifier/"

echo
echo "building from: $TMP"
podman build -t "$TAG" -f "$TMP/commentarr/Dockerfile" "$TMP"
echo
echo "built: $TAG"
