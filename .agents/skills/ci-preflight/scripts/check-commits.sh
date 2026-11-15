#!/usr/bin/env bash
# Replays the "Conventional commits" job of pr-title.yml on origin/main..HEAD.
set -euo pipefail
cd "${1:-.}"
wf=.github/workflows/pr-title.yml
types='build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test'
scopes=$(sed -n "s/.*scopes='\(.*\)'/\1/p" "$wf")
skip=$(grep -oE '[0-9a-f]{40}' "$wf" | tr '\n' ' ')
pattern="^(${types})(\((${scopes})\))?!?: [^A-Z ].*[^. ]$"
bad=0
while IFS= read -r line; do
	sha="${line%% *}"
	subject="${line#* }"
	case "$subject" in Merge\ * | Revert\ \"* | Potential\ fix\ for\ *) continue ;; esac
	case " $skip " in *" $sha "*) continue ;; esac
	if [ "${#subject}" -gt 100 ] || ! printf '%s\n' "$subject" | grep -qE "$pattern"; then
		echo "BAD ${sha:0:9} (${#subject}) $subject"
		bad=1
	fi
done < <(git log --format='%H %s' origin/main..HEAD)
[ "$bad" -eq 0 ] && echo "commits: ok"
exit "$bad"
