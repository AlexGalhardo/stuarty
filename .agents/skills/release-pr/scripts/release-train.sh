#!/usr/bin/env bash
# The release train (ADR-0048): one open release pull request at a time, on branch
# release/vX.Y.Z from main. Approved work is added to it as linear commits; when it holds about
# 90 commits it is cut (changelog, chore(release) commit), rebase-merged into main, and the
# next release branch and pull request are opened at once.
#
#   release-train.sh status                 the open release branch, its pull request and size
#   release-train.sh integrate <ref>...     cherry-pick the work of <ref> onto the release branch;
#                                           cuts on its own when the branch reaches MAX commits
#   release-train.sh cut [--force]          cut, merge and open the next release (needs MIN
#                                           commits unless --force)
#
# Run it in the main checkout, on the open release branch. It never force-pushes, never pushes
# main and never pushes a tag: the GitHub release of each cut carries its notes, created with
# the release workflow paused (ADR-0048 item 5).
set -euo pipefail

MIN=${RELEASE_TRAIN_MIN:-88}
MAX=${RELEASE_TRAIN_MAX:-94} # the cut adds two commits (version stamp, chore(release)): 96 at most
URL=https://github.com/AlexGalhardo/stuarty

die() {
	echo "release-train: $*" >&2
	exit 1
}

branch() { git rev-parse --abbrev-ref HEAD; }

version_of() { # release/v0.43.0 or release/v0.42.0-r2 -> 0.43.0
	sed -n 's#^release/v\([0-9]*\.[0-9]*\.[0-9]*\).*#\1#p' <<<"$1"
}

require_release_branch() {
	local b
	b=$(branch)
	[ -n "$(version_of "$b")" ] || die "the checkout is on $b, not on a release/vX.Y.Z branch"
	# Unstaged local edits may stay (a cherry-pick that touches one of those files stops safely).
	git diff --cached --quiet || die "the index has staged changes"
	git fetch -q origin main
	git merge-base --is-ancestor origin/main HEAD || die "$b does not start from origin/main"
}

count() { git rev-list --count origin/main..HEAD; }

pr_of() { gh pr list --head "$1" --state open --json number --jq '.[0].number // empty'; }

status() {
	local b
	b=$(branch)
	git fetch -q origin main
	echo "branch=$b version=$(version_of "$b") commits=$(count) merges=$(git rev-list --merges --count origin/main..HEAD) pr=$(pr_of "$b")"
}

# The commits of <ref> that are not on main or on any release branch, oldest first, without
# merges, and without those whose patch is already on the release branch.
pending_commits() {
	local ref=$1
	# shellcheck disable=SC2046 # one argument per release branch
	git rev-list --reverse --topo-order --no-merges "$ref" --not HEAD origin/main \
		$(git for-each-ref --format='%(refname)' refs/remotes/origin/release/) |
		while read -r c; do
			# Already on the line by patch, or picked before (-x records it; a commit whose conflict
			# was resolved has another patch id, so only the record finds it).
			[ "$(git cherry HEAD "$c" "$c^" 2>/dev/null | cut -c1)" = "-" ] && continue
			[ -n "$(git log -1 --format=%H --fixed-strings --grep="cherry picked from commit $c" HEAD)" ] && continue
			echo "$c"
		done
}

integrate() {
	[ $# -gt 0 ] || die "integrate needs at least one ref"
	require_release_branch
	local ref c
	for ref in "$@"; do
		for c in $(pending_commits "$ref"); do
			if ! git cherry-pick -x "$c" >/dev/null; then
				die "conflict on $(git log -1 --format='%h %s' "$c"): resolve it, run git cherry-pick --continue, then run integrate again"
			fi
			if [ "$(count)" -ge "$MAX" ]; then
				cut_release
			fi
		done
		echo "integrated $ref: $(count) commits on $(branch)"
	done
	git push -q origin "$(branch)"
	ensure_pr
}

# Writes the section of version $1 for the commits since the last release on main, replacing a
# section of the same version that an earlier cut of this branch wrote.
write_changelog() {
	local v=$1 last prev range section
	last=$(git log origin/main -1 --format=%H --grep='^chore(release): v' || true)
	prev=$(git log -1 --format=%s "${last:-HEAD}" | sed -n 's/^chore(release): \(v[0-9.]*\)$/\1/p')
	range="${last:+$last..}HEAD"
	section=$(mktemp)
	git cliff --config .config/cliff.toml "$range" --tag "v$v" --strip all >"$section" 2>/dev/null
	grep -q '^### ' "$section" || printf '## [%s] - %s\n\n### Changed\n\n- Maintenance release (no user-facing changes).\n' "$v" "$(date +%F)" >"$section"
	awk -v sec="$section" -v ver="$v" -v prev="${prev:-v0.29.0}" -v url="$URL" '
		index($0, "## [" ver "]") == 1 { skipping = 1; next }
		skipping && /^## \[/ { skipping = 0 }
		skipping { next }
		index($0, "[" ver "]: ") == 1 { next }
		/^## \[Unreleased\]/ && !done { print; print ""; while ((getline l < sec) > 0) { print l; last = l } if (last != "") print ""; done = 1; blank = 1; next }
		blank && /^$/ { blank = 0; next }
		{ blank = 0 }
		/^\[Unreleased\]: / { print "[Unreleased]: " url "/compare/v" ver "...HEAD"; print "[" ver "]: " url "/compare/" prev "...v" ver; next }
		{ print }
	' CHANGELOG.md >CHANGELOG.md.new
	mv CHANGELOG.md.new CHANGELOG.md
	rm -f "$section"
}

# Stamps the version into the agent docs (the one version of the binary: `version`, `doctor` and
# the agent docs read it) and regenerates them, in its own commit.
stamp_version() {
	local v=$1 f=web/app/agentdocs/agentdocs.go
	grep -q "^var Version = \"$v\"$" "$f" && return 0
	sed -i "s/^var Version = \".*\"$/var Version = \"$v\"/" "$f"
	go run ./web/cmd/gen-agent-docs >/dev/null
	git add "$f" web/app/agentdocs/out
	git commit -q -m "docs(cli): stamp the agent docs with version $v" -m "Refs: release $v"
}

pr_body() { # $1 version, $2 file to write
	local v=$1
	{
		echo "Release **v$v**: $(count) commits, linear (no merge commit), rebase-merged by the release train (ADR-0048)."
		echo
		echo "Approved work is added here as it is finished; the release is cut, rebase-merged and the next release pull request opened when it holds about 90 commits."
		echo
		git show HEAD:CHANGELOG.md | awk -v v="$v" 'index($0, "## [" v "]") == 1 { on = 1; print; next } on && /^## \[/ { exit } on'
	} >"$2"
}

cut_release() {
	local b v pr n next body
	b=$(branch)
	v=$(version_of "$b")
	n=$(count)
	[ "$n" -le 99 ] || die "$b holds $n commits: GitHub refuses a rebase-merge above 100; split it first"
	[ "$(git rev-list --merges --count origin/main..HEAD)" = 0 ] || die "$b has a merge commit"
	stamp_version "$v"
	write_changelog "$v"
	git add CHANGELOG.md
	git commit -q -m "chore(release): v$v"
	git push -q origin "$b"
	pr=$(pr_of "$b")
	body=$(mktemp)
	pr_body "$v" "$body"
	if [ -z "$pr" ]; then
		pr=$(gh pr create --base main --head "$b" --title "chore(release): v$v" --body-file "$body" | sed 's#.*/##')
	else
		# gh pr edit can fail on an unrelated GraphQL field of the repository; the body is only text.
		gh pr edit "$pr" --title "chore(release): v$v" --body-file "$body" >/dev/null ||
			echo "release-train: the body of #$pr was not updated" >&2
	fi
	gh pr merge "$pr" --rebase --admin
	until git fetch -q origin main && [ "$(git rev-parse 'origin/main^{tree}')" = "$(git rev-parse 'HEAD^{tree}')" ]; do sleep 5; done
	echo "released v$v: #$pr merged into main ($(git rev-parse --short origin/main))"
	publish_notes "$v"

	# The next release starts at main; its pull request opens with its first commit (GitHub opens
	# none for a branch equal to its base, and an empty commit breaks a rebase-merge).
	next="0.$(($(cut -d. -f2 <<<"$v") + 1)).0"
	git switch -q -c "release/v$next" origin/main
	git push -q -u origin "release/v$next"
	echo "next release: release/v$next (its pull request opens with the first integrated commit)"
	rm -f "$body"
}

# Publishes the GitHub release vX.Y.Z with its changelog section as notes, on the release commit
# of main. The release workflow is paused meanwhile, so the tag builds and publishes nothing
# while CI is off (ADR-0048 item 5).
publish_notes() {
	local v=$1 notes
	notes=$(mktemp)
	git show origin/main:CHANGELOG.md | awk -v v="$v" 'index($0, "## [" v "]") == 1 { on = 1; next } on && /^## \[/ { exit } on' >"$notes"
	gh workflow disable release
	gh release create "v$v" --target "$(git rev-parse origin/main)" --title "v$v" --notes-file "$notes" >/dev/null || echo "release-train: the GitHub release v$v was not created" >&2
	gh workflow enable release
	rm -f "$notes"
	echo "published the GitHub release v$v"
	# The site reads the latest release at build time (the landing buttons): rebuild and publish it now.
	gh workflow run site.yml --ref main || echo "release-train: the site workflow was not dispatched" >&2
}

# Opens the pull request of the release branch when it has commits and none is open.
ensure_pr() {
	local b v body
	b=$(branch)
	v=$(version_of "$b")
	[ "$(count)" -gt 0 ] && [ -z "$(pr_of "$b")" ] || return 0
	body=$(mktemp)
	pr_body "$v" "$body"
	echo "opened $(gh pr create --base main --head "$b" --title "chore(release): v$v" --body-file "$body")"
	rm -f "$body"
}

case "${1:-}" in
status) status ;;
integrate)
	shift
	integrate "$@"
	;;
cut)
	require_release_branch
	[ "${2:-}" = "--force" ] || [ "$(count)" -ge "$MIN" ] || die "$(count) commits: cut at $MIN or more, or pass --force"
	cut_release
	;;
*) die "usage: release-train.sh status | integrate <ref>... | cut [--force]" ;;
esac
