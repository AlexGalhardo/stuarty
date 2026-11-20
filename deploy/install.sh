#!/usr/bin/env bash
# Installs Stuarty behind Caddy on this host (SDD-009 section 21.3).
#
#   bash deploy/install.sh [--domain stuarty.example.com] [--email you@example.com]
#                          [--version X.Y.Z] [--no-start]
#
# Run it from a checkout of a release tag, on a host with Docker 24+ and Compose 2.20+, with the
# DNS record of the domain pointing to this host and ports 80 and 443 open. It builds the image
# from this checkout, creates the keys and the first administrator password, writes deploy/.env,
# starts the stack and waits until https://<domain>/up answers.
#
# Running it again changes nothing and never replaces a key. --no-start only writes the
# configuration and the keys: use it on a new host before restore.sh, after putting your old
# master key at deploy/secrets/master_key.
set -euo pipefail

# shellcheck source=deploy/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

domain="" email="" version="" start=1
while [ $# -gt 0 ]; do
	case "$1" in
		--domain) domain="${2:?--domain needs a value}"; shift 2 ;;
		--email) email="${2:?--email needs a value}"; shift 2 ;;
		--version) version="${2:?--version needs a value}"; shift 2 ;;
		--no-start) start=0; shift ;;
		-h | --help) sed -n '2,12p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
		*) die "unknown argument: $1 (see --help)" ;;
	esac
done

check_docker

ask() { # ask VAR PROMPT
	local var="$1" prompt="$2" answer=""
	[ -t 0 ] || die "$prompt is needed: pass it as a flag, or run this in a terminal"
	read -r -p "$prompt: " answer
	printf -v "$var" '%s' "$answer"
}

created_env=0
if [ -f "$ENV_FILE" ]; then
	# An existing installation is never changed. A flag that disagrees with it is a mistake to report.
	[ -z "$domain" ] || [ "$domain" = "$(env_get STUARTY_DOMAIN)" ] || die "this host is already installed for $(env_get STUARTY_DOMAIN); edit $ENV_FILE by hand to change it"
	domain="$(env_get STUARTY_DOMAIN)"
	email="$(env_get STUARTY_ADMIN_EMAIL)"
	[ -n "$domain" ] && [ -n "$email" ] || die "$ENV_FILE is incomplete: remove it to start again (the keys in $SECRETS_DIR are kept)"
	[ -z "$version" ] || [ "$version" = "$(env_get STUARTY_VERSION)" ] || die "this host runs $(env_get STUARTY_VERSION): use deploy/update.sh $version to change the version"
	version="$(env_get STUARTY_VERSION)"
else
	[ -n "$domain" ] || ask domain "Domain of this server (its DNS record must point here)"
	[ -n "$email" ] || ask email "E-mail of the first administrator (also given to the certificate authority)"
	printf '%s' "$domain" | grep -Eq '^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)+$' || die "'$domain' is not a domain name such as stuarty.example.com"
	printf '%s' "$email" | grep -Eq '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$' || die "'$email' is not an e-mail address"
	if [ -z "$version" ]; then
		version="$(git -C "$DEPLOY_DIR/.." describe --tags --exact-match 2>/dev/null || true)"
		version="${version#v}"
	fi
	[ -n "$version" ] || die "this checkout is not at a release tag: run git checkout vX.Y.Z first, or pass --version X.Y.Z"
	printf '%s' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || die "'$version' is not a version such as 0.30.0"
	created_env=1
fi

# The keys: created only when absent, so a second run (or a host prepared for a restore) keeps them.
mkdir -p "$SECRETS_DIR"
chmod 700 "$SECRETS_DIR"
created_password=0
make_secret() { # make_secret NAME VALUE
	local file="$SECRETS_DIR/$1"
	if [ -e "$file" ]; then
		[ -s "$file" ] || die "$file exists and is empty: put the key in it, or remove it"
		return 1
	fi
	# 0644 on purpose: the container reads the file as uid 10001. The directory is 0700, so no
	# other user of this host can reach it.
	(umask 022 && printf '%s\n' "$2" >"$file")
	chmod 644 "$file"
	return 0
}
make_secret master_key "$(rand_b64)" && say "created the master key: $SECRETS_DIR/master_key" || true
make_secret app_key "$(rand_b64)" || true
if make_secret admin_password "$(rand_hex)"; then created_password=1; fi

if [ "$created_env" = 1 ]; then
	(
		umask 077
		{
			printf 'STUARTY_DOMAIN=%s\n' "$domain"
			printf 'STUARTY_ADMIN_EMAIL=%s\n' "$email"
			printf 'STUARTY_ACME_EMAIL=%s\n' "$email"
			printf 'STUARTY_VERSION=%s\n' "$version"
			printf 'STUARTY_TLS_DIRECTIVE=%s\n' "${STUARTY_TLS_DIRECTIVE:-}"
		} >"$ENV_FILE"
	)
	chmod 600 "$ENV_FILE"
fi

dc config --quiet || die "the compose file is not valid with $ENV_FILE"

if [ "$start" = 0 ]; then
	say "configuration and keys written; nothing was started."
	say "master key: $SECRETS_DIR/master_key (a backup does not contain it: keep a copy elsewhere)"
	exit 0
fi

if ! docker image inspect "stuarty:$version" >/dev/null 2>&1; then
	say "building stuarty:$version from this checkout (the first build takes several minutes)..."
	dc build stuarty
fi

dc up -d

say "waiting for Stuarty to be healthy..."
wait_healthy "${STUARTY_HEALTH_TIMEOUT:-180}" || {
	dc logs --tail 40 stuarty >&2 || true
	die "the stuarty container did not become healthy: see the log above"
}

say "waiting for https://$domain (the first certificate can take a minute)..."
wait_https "${STUARTY_HTTPS_TIMEOUT:-180}" || {
	dc logs --tail 20 caddy >&2 || true
	die "https://$domain did not answer. Check that its DNS record points to this host and that ports 80 and 443 are open, then run this script again: it is safe to repeat."
}

say ""
say "Stuarty $version is running."
say "  address:        https://$domain"
say "  administrator:  $email"
if [ "$created_password" = 1 ]; then
	say "  password:       $(cat "$SECRETS_DIR/admin_password")"
	say "                  (also in $SECRETS_DIR/admin_password; it only creates the first account: change it after you sign in)"
else
	say "  password:       the first one is in $SECRETS_DIR/admin_password (it is not shown again)"
fi
say "  master key:     $SECRETS_DIR/master_key"
say ""
say "Keep a copy of the master key outside this server. A backup does not contain it, and without it the"
say "connected accounts of a restored backup cannot be opened. Back up with: bash deploy/backup.sh"
