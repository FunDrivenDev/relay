#!/usr/bin/env bash
# Usage: release.sh <snapshot|publish>.
#   snapshot  builds the Linux archives, checksums.txt and the .deb and .rpm
#             packages into dist/, publishing nothing.
#   publish   publishes the vX.Y.Z tag on HEAD as a GitHub release (GITHUB_TOKEN
#             must be set); scripts/formula.sh X.Y.Z then prints the Homebrew
#             formula for FunDrivenDev/homebrew-tap (docs/development.md).
set -euo pipefail

case "$1" in
    snapshot) goreleaser release --snapshot --clean ;;
    publish)
        tag=$(git describe --tags --exact-match HEAD 2>/dev/null) || {
            echo "no tag on HEAD: git tag vX.Y.Z && git push origin vX.Y.Z" >&2
            exit 1
        }
        [[ -z "$(git status --porcelain)" ]] || {
            echo "worktree not clean" >&2
            exit 1
        }
        [[ -n "${GITHUB_TOKEN:-}" ]] || {
            echo "GITHUB_TOKEN is not set" >&2
            exit 1
        }
        echo "publishing $tag" >&2
        goreleaser release --clean
        ;;
    *)
        echo "unknown mode $1: snapshot or publish" >&2
        exit 2
        ;;
esac
