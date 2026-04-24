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
#
# Multi-arch: pass BUILD_PLATFORMS=linux/amd64,linux/arm64 (default is
# the host's native platform) and BUILDER=podman or docker. buildx is
# required for cross-platform builds; podman manages that transparently
# via QEMU emulation.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SIBLING="$(cd "$REPO_ROOT/../commentary-classifier" && pwd)"
TAG="${IMAGE_TAG:-commentarr:dev}"
BUILDER="${BUILDER:-podman}"
PLATFORMS="${BUILD_PLATFORMS:-}"

echo "commentarr repo:       $REPO_ROOT"
echo "classifier sibling:    $SIBLING"
echo "image tag:             $TAG"
echo "builder:               $BUILDER"
echo "platforms:             ${PLATFORMS:-<native>}"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# rsync preserves permissions and excludes VCS / build artifacts
rsync -a --exclude '.git' --exclude 'vendor' --exclude 'bin' \
      --exclude '*.test' --exclude '/commentarr' \
      --exclude 'node_modules' --exclude 'web/dist' \
      "$REPO_ROOT/" "$TMP/commentarr/"
rsync -a --exclude '.git' --exclude 'vendor' --exclude 'bin' \
      --exclude '*.test' \
      "$SIBLING/" "$TMP/commentary-classifier/"

echo
echo "building from: $TMP"
if [[ -n "$PLATFORMS" ]]; then
  "$BUILDER" build --platform "$PLATFORMS" -t "$TAG" \
    -f "$TMP/commentarr/Dockerfile" "$TMP"
else
  "$BUILDER" build -t "$TAG" -f "$TMP/commentarr/Dockerfile" "$TMP"
fi
echo
echo "built: $TAG"
