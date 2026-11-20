#!/usr/bin/env bash
# Shared functions of the scripts of deploy/ (SDD-009 section 21). It is sourced, never run.
# Nothing here opens a connection other than to the Docker daemon and, to check the
# address you chose, to that address.

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$DEPLOY_DIR/.env"
SECRETS_DIR="$DEPLOY_DIR/secrets"

say() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

# dc runs docker compose on the stack of this folder.
dc() { docker compose --project-directory "$DEPLOY_DIR" -f "$DEPLOY_DIR/compose.yml" "$@"; }

# version_ge A B: success when version A is B or later (dotted numbers).
version_ge() { [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n 1)" = "$2" ]; }

# check_docker needs Docker 24 or later and Compose 2.20 or later.
check_docker() {
	command -v docker >/dev/null 2>&1 || die "docker is not installed: see https://docs.docker.com/engine/install/"
	command -v curl >/dev/null 2>&1 || die "curl is not installed: it checks that https://<domain>/up answers"
	local dv cv
	dv="$(docker version --format '{{.Server.Version}}' 2>/dev/null)" || die "cannot talk to the Docker daemon: is it running, and may this user use it?"
	version_ge "${dv%%-*}" 24.0 || die "Docker $dv is too old: 24 or later is needed"
	cv="$(docker compose version --short 2>/dev/null)" || die "the Docker Compose plugin is missing: see https://docs.docker.com/compose/install/linux/"
	version_ge "${cv#v}" 2.20 || die "Docker Compose $cv is too old: 2.20 or later is needed"
}

# env_get KEY prints the value of KEY in .env, or nothing.
env_get() {
	[ -f "$ENV_FILE" ] || return 0
	local line
	line="$(grep -E "^$1=" "$ENV_FILE" | tail -n 1 || true)"
	printf '%s' "${line#*=}"
}

# env_set KEY VALUE rewrites or appends one line of .env, keeping its mode.
env_set() {
	local tmp
	tmp="$(mktemp "$DEPLOY_DIR/.env.XXXXXX")"
	if [ -f "$ENV_FILE" ]; then
		grep -v -E "^$1=" "$ENV_FILE" >"$tmp" || true
	fi
	printf '%s=%s\n' "$1" "$2" >>"$tmp"
	chmod 600 "$tmp"
	mv "$tmp" "$ENV_FILE"
}

require_env() {
	[ -f "$ENV_FILE" ] || die "$ENV_FILE does not exist: run deploy/install.sh first"
	[ -n "$(env_get STUARTY_DOMAIN)" ] || die "STUARTY_DOMAIN is not set in $ENV_FILE"
	[ -n "$(env_get STUARTY_VERSION)" ] || die "STUARTY_VERSION is not set in $ENV_FILE"
}

# wait_healthy SECONDS: the stuarty container reports healthy.
wait_healthy() {
	local limit="$1" cid status i
	for ((i = 0; i < limit; i += 2)); do
		cid="$(dc ps -q stuarty 2>/dev/null || true)"
		if [ -n "$cid" ]; then
			status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$cid" 2>/dev/null || true)"
			[ "$status" = healthy ] && return 0
			[ "$status" = unhealthy ] && return 1
		fi
		sleep 2
	done
	return 1
}

# wait_https SECONDS: /up answers through Caddy over HTTPS (a certificate may take a minute to be issued).
# STUARTY_CURL_OPTS is for a test with `tls internal` (for example -k --resolve ...).
wait_https() {
	local limit="$1" domain i
	domain="$(env_get STUARTY_DOMAIN)"
	for ((i = 0; i < limit; i += 3)); do
		# shellcheck disable=SC2086
		if curl -fsS -o /dev/null --max-time 10 ${STUARTY_CURL_OPTS:-} "https://$domain/up" 2>/dev/null; then
			return 0
		fi
		sleep 3
	done
	return 1
}

rand_b64() { head -c 32 /dev/urandom | base64 | tr -d '\n'; }
rand_hex() { od -An -N24 -tx1 /dev/urandom | tr -d ' \n'; }
