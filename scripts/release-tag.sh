#!/bin/sh
# Classifies a git tag as a Release Tag (see docs/ci.md, "Release tags").
#
# Usage: sh scripts/release-tag.sh <tag>
#
# Prints three key=value lines, usable as GitHub Actions step outputs
# (>> "$GITHUB_OUTPUT"), as a GitLab dotenv report, or with `eval` in a shell:
#
#   release=true|false      the tag is a semantic version (optional leading v)
#   prerelease=true|false   the version has a pre-release part (X.Y.Z-...)
#   environment=dev|stage|prod, or empty when the tag deploys nowhere
#
#   X.Y.Z, vX.Y.Z      release=true  prerelease=false environment=prod
#   X.Y.Z-beta[.N]     release=true  prerelease=true  environment=stage
#   X.Y.Z-alpha[.N]    release=true  prerelease=true  environment=dev
#   X.Y.Z-<other>      release=true  prerelease=true  environment=      (release only)
#   anything else      release=false prerelease=false environment=      (ignored)
#
# Build metadata (+...) is accepted and does not change the result.
set -eu

if [ "$#" -ne 1 ]; then
	echo "usage: $0 <tag>" >&2
	exit 2
fi

num='(0|[1-9][0-9]*)'
ident='[0-9A-Za-z-]+'
semver="^v?$num\\.$num\\.$num(-$ident(\\.$ident)*)?(\\+$ident(\\.$ident)*)?\$"

release=false
prerelease=false
environment=

if printf '%s\n' "$1" | grep -Eq "$semver"; then
	release=true
	version=${1#v}
	version=${version%%+*}
	case $version in
	*-*)
		prerelease=true
		pre=${version#*-}
		if printf '%s\n' "$pre" | grep -Eq '^beta(\.[0-9]+)?$'; then
			environment=stage
		elif printf '%s\n' "$pre" | grep -Eq '^alpha(\.[0-9]+)?$'; then
			environment=dev
		fi
		;;
	*)
		environment=prod
		;;
	esac
fi

printf 'release=%s\nprerelease=%s\nenvironment=%s\n' "$release" "$prerelease" "$environment"
