#!/bin/sh
# Bump the Version (semver) and Build (monotonic counter) fields in
# FyneApp.toml and print the new version to stdout.
#
# Usage: scripts/bump_version.sh [major|minor|patch] [--dry-run]
#
# --dry-run prints the version the bump would produce and touches nothing,
# so callers can check whether the resulting tag already exists before
# writing to the working tree.

set -eu

part=${1:-patch}
dry_run=${2:-}
toml=FyneApp.toml

if [ ! -f "$toml" ]; then
	echo "$toml not found (run from the repository root)" >&2
	exit 1
fi

version=$(sed -nE 's/^Version = "(.*)"/\1/p' "$toml")
build=$(sed -nE 's/^Build = ([0-9]+)/\1/p' "$toml")

if [ -z "$version" ] || [ -z "$build" ]; then
	echo "could not read Version/Build from $toml" >&2
	exit 1
fi

major=$(echo "$version" | cut -d. -f1)
minor=$(echo "$version" | cut -d. -f2)
patch=$(echo "$version" | cut -d. -f3)

case $part in
major) major=$((major + 1)); minor=0; patch=0 ;;
minor) minor=$((minor + 1)); patch=0 ;;
patch) patch=$((patch + 1)) ;;
*)
	echo "Unknown part '$part' (want major|minor|patch)" >&2
	exit 1
	;;
esac

new_version=$major.$minor.$patch

if [ "$dry_run" = "--dry-run" ]; then
	echo "$new_version"
	exit 0
fi

new_build=$((build + 1))

sed -i.bak -E "s/^Version = \".*\"/Version = \"$new_version\"/" "$toml"
sed -i.bak -E "s/^Build = [0-9]+/Build = $new_build/" "$toml"
rm -f "$toml.bak"

echo "Bumped version $version -> $new_version (build $build -> $new_build)" >&2
echo "$new_version"
