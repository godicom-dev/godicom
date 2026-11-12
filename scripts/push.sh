#!/usr/bin/env bash
#
# Push refs, then prove the remote actually moved.
#
# Why this exists. A push over a flaky proxy can die in the protocol
# ("RPC failed; curl 52 Empty reply from server") and the next attempt can
# answer "Everything up-to-date" while the remote still holds the old commit.
# Both the exit status and that message describe what git believed, not what
# GitHub stored. The only evidence is what ls-remote reports afterwards, so
# that is what this script trusts.
#
# Usage:
#   scripts/push.sh              # push the current branch
#   scripts/push.sh main v1.2.3  # push a branch and a tag
#
# Environment:
#   REMOTE    remote name              (default: origin)
#   ATTEMPTS  tries per ref            (default: 5)
#   GIT_PROXY proxy URL for git only   (default: unset -- direct)
#
# The proxy is passed as `-c http.proxy=...` rather than through HTTP_PROXY.
# Environment-variable proxies apply to every request git makes and are what
# produced the 502 / curl 52 failures; the per-command setting is scoped to
# this invocation and leaves the environment alone.
set -uo pipefail

remote=${REMOTE:-origin}
attempts=${ATTEMPTS:-5}
refs=("$@")

if [ ${#refs[@]} -eq 0 ]; then
	refs=("$(git rev-parse --abbrev-ref HEAD)")
fi

git_cmd=(git -c http.postBuffer=524288000 -c http.version=HTTP/1.1)
if [ -n "${GIT_PROXY:-}" ]; then
	git_cmd+=(-c "http.proxy=${GIT_PROXY}")
fi

# remote_commit prints the commit a ref resolves to on the remote, or nothing
# when the ref is absent. An annotated tag appears twice: once as the tag
# object and once peeled to the commit, so the peeled line wins when present.
remote_commit() {
	local ref=$1 out peeled plain
	out=$("${git_cmd[@]}" ls-remote "$remote" "refs/heads/$ref" "refs/tags/$ref" "refs/tags/$ref^{}" 2>/dev/null) || return 1
	peeled=$(printf '%s\n' "$out" | awk '/\^\{\}$/{print $1; exit}')
	if [ -n "$peeled" ]; then
		printf '%s\n' "$peeled"
		return 0
	fi
	plain=$(printf '%s\n' "$out" | awk 'NF>=2{print $1; exit}')
	printf '%s\n' "$plain"
}

failed=0
for ref in "${refs[@]}"; do
	local_sha=$(git rev-parse --verify "$ref^{commit}" 2>/dev/null) || {
		printf 'push: %s does not resolve to a commit\n' "$ref" >&2
		failed=1
		continue
	}

	pushed=0
	for ((attempt = 1; attempt <= attempts; attempt++)); do
		printf 'push: %s -> %s (attempt %d/%d)\n' "$ref" "$remote" "$attempt" "$attempts"
		"${git_cmd[@]}" push "$remote" "$ref"
		status=$?

		# git's own verdict is not enough; ask the remote what it holds.
		if got=$(remote_commit "$ref") && [ "$got" = "$local_sha" ]; then
			printf 'push: %s verified at %s\n' "$ref" "${local_sha:0:8}"
			pushed=1
			break
		fi

		if [ "$status" -ne 0 ]; then
			printf 'push: attempt %d failed (exit %d)\n' "$attempt" "$status" >&2
		else
			printf 'push: attempt %d reported success but %s is still %s, want %s\n' \
				"$attempt" "$remote" "${got:0:8}" "${local_sha:0:8}" >&2
		fi
		if [ "$attempt" -lt "$attempts" ]; then
			sleep 3
		fi
	done

	if [ "$pushed" -eq 0 ]; then
		printf 'push: %s NOT pushed -- the remote does not have %s\n' "$ref" "${local_sha:0:8}" >&2
		failed=1
	fi
done

exit "$failed"
