#!/usr/bin/env bash
# The verdict of scripts/no-egress.sh on a set of strace logs:
#
#   bash scripts/no-egress-scan.sh <strace log>...
#
# Exit 0 when every inet socket address is loopback (127.0.0.0/8, ::1, ::ffff:127.x) and at least one
# loopback call is present (an empty trace means strace saw nothing and the result is void). Exit 1
# otherwise. Unix sockets carry no sa_family=AF_INET and are ignored. TestSecurity_T10_EgressScanner
# runs this file on fixtures.
set -euo pipefail

[ "$#" -gt 0 ] || { echo "no-egress: error: no trace to scan" >&2; exit 1; }

loopback_re='inet_addr\("127\.[0-9.]+"\)|inet_pton\(AF_INET6, "(::1|::ffff:127\.[0-9.]+)"(, &sin6_addr)?\)'
net_lines="$(grep -hE 'sa_family=AF_INET6?' "$@" || true)"
bad="$(printf '%s\n' "$net_lines" | grep -vE "^$|$loopback_re" || true)"
if [ -n "$bad" ]; then
	echo "no-egress: socket addresses that are not loopback:" >&2
	printf '%s\n' "$bad" >&2
	echo "no-egress: error: a non-loopback address was used" >&2
	exit 1
fi
good="$(printf '%s\n' "$net_lines" | grep -cE "$loopback_re" || true)"
[ "$good" -gt 0 ] || { echo "no-egress: error: the traces hold no loopback connect: strace saw nothing, the result is void" >&2; exit 1; }
echo "no-egress: ok: $good loopback socket calls, no other address, in $# traced processes" >&2
