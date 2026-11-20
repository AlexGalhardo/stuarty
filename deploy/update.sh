#!/usr/bin/env bash
# Updates this installation to another release (SDD-009 section 21.5).
#
#   bash deploy/update.sh <version>        for example: bash deploy/update.sh 0.31.0
#
# Run it from the checkout you installed from. It backs up first, checks out the tag, builds the image
# on this host, starts it (which runs the migrations) and waits for it to be healthy. When it is not
# healthy within STUARTY_HEALTH_TIMEOUT seconds (default 120) it puts the previous checkout, the previous
# image and the backup back, and exits 1. Exit 2 means that even the rollback failed: the message names
# the backup to restore by hand. It contacts only the git remote of the checkout and never pulls an image.
set -euo pipefail

# shellcheck source=deploy/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# The whole script is one function that runs after bash has read it: the checkout below replaces this
# file with another version while it runs.
main() {
	local new="${1:-}"
	case "$new" in -h | --help) sed -n '2,10p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; return 0 ;; esac
	new="${new#v}"
	[ -n "$new" ] || die "usage: bash deploy/update.sh <version>"
	printf '%s' "$new" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || die "'$new' is not a version such as 0.31.0"

	local repo="$DEPLOY_DIR/.."
	check_docker
	require_env
	git -C "$repo" rev-parse --git-dir >/dev/null 2>&1 || die "$repo is not a git checkout: update from the checkout you installed from"
	[ -z "$(git -C "$repo" status --porcelain --untracked-files=no)" ] || die "the checkout has uncommitted changes: commit or discard them first"

	local current
	current="$(env_get STUARTY_VERSION)"
	if [ "$current" = "$new" ]; then
		say "already running $new: nothing to do"
		return 0
	fi
	docker image inspect "stuarty:$current" >/dev/null 2>&1 || die "the image stuarty:$current of the running version is not on this host, so there is nothing to roll back to"

	git -C "$repo" fetch --tags --quiet 2>/dev/null || warn "could not fetch tags from the remote: using the tags this checkout already has"
	git -C "$repo" rev-parse -q --verify "refs/tags/v$new^{commit}" >/dev/null || die "there is no tag v$new in this checkout"
	local prev_head
	prev_head="$(git -C "$repo" rev-parse HEAD)"

	say "1/4 backing up before the update..."
	bash "$DEPLOY_DIR/backup.sh" >&2
	local backup
	backup="$(ls -1t "$DEPLOY_DIR"/backups/stuarty-backup-*.tar 2>/dev/null | head -n 1 || true)"
	[ -n "$backup" ] || die "the backup left no file: the update was not started"

	say "2/4 building stuarty:$new..."
	git -C "$repo" checkout --quiet --detach "v$new"
	env_set STUARTY_VERSION "$new"
	if ! dc build stuarty; then
		git -C "$repo" checkout --quiet --detach "$prev_head"
		env_set STUARTY_VERSION "$current"
		die "the build of $new failed: nothing that was running was changed (still $current)"
	fi

	say "3/4 starting $new (the migrations run now)..."
	dc up -d
	if wait_healthy "${STUARTY_HEALTH_TIMEOUT:-120}" && wait_https "${STUARTY_HEALTH_TIMEOUT:-120}"; then
		say "4/4 $new is healthy."
		say "updated $current -> $new. The backup made before is $backup"
		return 0
	fi

	warn "$new did not become healthy: rolling back to $current and restoring the backup"
	dc logs --tail 30 stuarty >&2 || true
	dc stop >/dev/null 2>&1 || true
	git -C "$repo" checkout --quiet --detach "$prev_head"
	env_set STUARTY_VERSION "$current"
	if bash "$DEPLOY_DIR/restore.sh" "$backup" --replace >&2 && dc up -d && wait_healthy "${STUARTY_HEALTH_TIMEOUT:-120}" && wait_https "${STUARTY_HEALTH_TIMEOUT:-120}"; then
		die "the update to $new failed its health check: $current is running again with the data of the backup $backup (what $new had written is kept in replaced-<time>/ of the volume)"
	fi
	printf 'error: the rollback to %s failed too. Your data is in the backup %s: restore it by hand with\n  bash deploy/restore.sh %s --replace\n  bash deploy/install.sh\n' "$current" "$backup" "$backup" >&2
	return 2
}

main "$@"
exit $?
