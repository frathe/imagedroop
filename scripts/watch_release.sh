#!/bin/sh
# Wait for the Release workflow run for TAG to appear, then watch it
# until it finishes. Non-interactive: never prompts to pick among runs
# (a tag push also starts CI on main, which is a different workflow).
#
# Usage: scripts/watch_release.sh TAG
#
# If gh is not installed the script prints a pointer and exits 0, matching
# the previous make-release behaviour.

set -eu

tag=${1:-}

if [ -z "$tag" ]; then
	echo "usage: scripts/watch_release.sh TAG" >&2
	exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
	echo "gh is not installed; watch $tag at the repository's Actions tab"
	exit 0
fi

# gh run watch with no ID prompts; disable prompts anyway so a future
# gh version cannot block make release on a TTY question.
export GH_PROMPT_DISABLED=1
export GH_NO_UPDATE_NOTIFIER=1

echo "Waiting for the Release workflow for $tag to start..."

run_id=
i=0
while [ "$i" -lt 30 ]; do
	run_id=$(gh run list --workflow=release.yml --branch "$tag" --limit 1 --json databaseId --jq '.[0].databaseId // empty')
	if [ -n "$run_id" ]; then
		break
	fi
	i=$((i + 1))
	sleep 2
done

if [ -z "$run_id" ]; then
	echo "No Release workflow run found for $tag" >&2
	exit 1
fi

echo "Watching Release run $run_id"
gh run watch "$run_id" --exit-status

if url=$(gh release view "$tag" --json url --jq .url 2>/dev/null) && [ -n "$url" ]; then
	echo "Published $url"
else
	echo "Workflow succeeded; the GitHub release for $tag should appear shortly."
fi
