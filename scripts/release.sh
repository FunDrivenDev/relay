#!/usr/bin/env bash
# Usage: release.sh snapshot | release.sh publish [--dry-run] [vX.Y.Z].
#   snapshot  builds the Linux archives, checksums.txt and the .deb and .rpm
#             packages into dist/, publishing nothing.
#   publish   releases origin/main the way the personal repos release
#             (toolbox's docs/release.md): the version is the one the commits
#             since the last v* tag call for (`toolbox release next`: breaking a
#             major, feat a minor, any other a patch; M-D3FJ:D6, M-HJGS:D03); a
#             version given must be that one, or an existing tag, whose release
#             a rerun finishes (a tag left without a published release must be
#             finished before a new version is computed). With none given and
#             origin/main already tagged, that tag's release is finished. The
#             commit is tagged, the tag pushed, and goreleaser publishes its
#             GitHub release from a temporary worktree of the tag (GITHUB_TOKEN,
#             else `gh auth token`). Nothing is committed: the tags hold the
#             versions. Its last line is the tag released, `{"tag": "vX.Y.Z"}`,
#             a dry run's included. scripts/formula.sh X.Y.Z then prints the
#             Homebrew formula for FunDrivenDev/homebrew-tap
#             (docs/development.md).
set -euo pipefail

mode=${1:-snapshot}
shift || true
case "$mode" in
    snapshot)
        goreleaser release --snapshot --clean
        exit 0
        ;;
    publish) ;;
    *)
        echo "unknown mode $mode: snapshot or publish" >&2
        exit 2
        ;;
esac

dry=""
version=""
for arg in "$@"; do
    case "$arg" in
        --dry-run) dry=1 ;;
        -*)
            echo "release: unknown option $arg: --dry-run" >&2
            exit 2
            ;;
        *) version=${arg#v} ;;
    esac
done
say() { echo "release: $*" >&2; }
run() { if [[ -n "$dry" ]]; then say "would run: $*"; else "$@"; fi; }

changes=$(git status --porcelain)
[[ -z "$changes" ]] || {
    echo "release: the working tree is not clean" >&2
    exit 1
}
# One release at a time on this machine, as toolbox's release.lock.
lock="$(git rev-parse --git-common-dir)/release.lock"
mkdir "$lock" 2>/dev/null || {
    echo "release: another release runs ($lock); remove it if none does" >&2
    exit 1
}
trap 'rmdir "$lock"' EXIT
git fetch --quiet --tags origin main
head=$(git rev-parse origin/main)
draft_of() { gh release view "$1" --json isDraft --jq .isDraft 2>/dev/null || true; }
released() { [[ "$(draft_of "$1")" == false ]]; }

if [[ -n "$version" ]] && git rev-parse --verify --quiet "refs/tags/v$version" >/dev/null; then
    # An existing tag: finish its release, built from the commit it tags.
    head=$(git rev-parse "v$version^{commit}")
else
    # origin/main already tagged: finish that release rather than tag it again.
    tagged_head=$(git tag --points-at "$head" --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -1)
    if [[ -n "$tagged_head" && -z "$version" ]]; then
        version=${tagged_head#v}
    else
        # A tag left without a published release is finished first, as toolbox's release_unfinished.
        last=$(git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' "$head" 2>/dev/null || true)
        if [[ -n "$last" ]] && ! released "$last"; then
            echo "release: $last has no published release: finish it first with \`just release publish $last\`" >&2
            exit 1
        fi
        computed=$(toolbox release next --repo . --at "$head")
        if [[ -z "$computed" ]]; then
            # Only a version given reaches this: with none, a head with no commit since its tag is tagged.
            echo "release: no commit since $last calls for a release, not v$version" >&2
            exit 2
        elif [[ -z "$version" ]]; then
            version=$computed
        elif [[ "$version" != "$computed" ]]; then
            echo "release: the commits since the last tag call for $computed, not $version" >&2
            exit 2
        fi
    fi
fi
tag="v$version"

if released "$tag"; then
    say "$tag is already released"
    echo "{\"tag\": \"$tag\"}"
    exit 0
fi
# goreleaser creates the release and uploads every asset itself: a draft left by
# a failed run is deleted so the rerun starts it again.
if [[ "$(draft_of "$tag")" == true ]]; then
    run gh release delete "$tag" --yes
fi

if ! git rev-parse --verify --quiet "refs/tags/$tag" >/dev/null; then
    run git tag --annotate "$tag" -m "relay $version" "$head"
fi
# Pushed whenever origin lacks it, so a rerun recovers a tag made but never pushed.
if ! git ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null; then
    run git push --quiet origin "$tag"
fi

if [[ -n "$dry" ]]; then
    say "would run: goreleaser release --clean, in a worktree of $tag"
else
    token=${GITHUB_TOKEN:-$(gh auth token)}
    tree=$(mktemp -d)
    trap 'git worktree remove --force "$tree" 2>/dev/null || rm -rf "$tree"; rmdir "$lock"' EXIT
    git worktree add --quiet --detach "$tree" "$tag"
    (cd "$tree" && GITHUB_TOKEN=$token goreleaser release --clean)
    url=$(gh release view "$tag" --json url --jq .url)
    say "published $tag: $url"
fi
echo "{\"tag\": \"$tag\"}"
