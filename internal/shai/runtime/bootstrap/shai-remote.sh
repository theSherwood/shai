#!/bin/sh
set -u

EX_USAGE=64
EX_UNAVAILABLE=69

endpoint=${SHAI_ALIAS_ENDPOINT-}
token=${SHAI_ALIAS_TOKEN-}
session_id=${SHAI_ALIAS_SESSION_ID-}
env_verbose=${SHAI_ALIAS_DEBUG-0}
cli_verbose=0

usage() {
	cat <<'EOF'
Usage:
  shai-remote list [--endpoint URL] [--token TOKEN] [--session ID] [--verbose]
  shai-remote call <name> [args...] [--endpoint URL] [--token TOKEN] [--session ID] [--verbose]
EOF
	exit "${1:-$EX_USAGE}"
}

log_err() {
	printf '%s\n' "$*" >&2
}

die() {
	status=$1
	shift
	log_err "$@"
	exit "$status"
}

require_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		die "$EX_UNAVAILABLE" "shai-remote: required command '$1' not found"
	fi
}

debug_enabled=0
if [ "${env_verbose}" != "0" ] && [ -n "${env_verbose}" ]; then
	debug_enabled=1
fi

debug() {
	if [ "$debug_enabled" -eq 1 ]; then
		log_err "shai-remote[debug]: $*"
	fi
}

build_payload_list() {
	printf '{"jsonrpc":"2.0","id":1,"method":"listTools"}'
}

build_payload_call() {
	call_name=$1
	shift
	jq -nc --arg alias "$call_name" --args '
		{jsonrpc:"2.0",id:99,method:"callTool",
		 params:{name:$alias,args:$ARGS.positional}}
	' -- "$@"
}

ensure_env() {
	if [ -z "${endpoint}" ]; then
		die 1 "shai-remote: missing SHAI_ALIAS_ENDPOINT (set env or use --endpoint)"
	fi
	if [ -z "${token}" ]; then
		die 1 "shai-remote: missing SHAI_ALIAS_TOKEN (set env or use --token)"
	fi
}

mcp_post() {
	payload=$1
	debug "payload: $payload"
	response=$(printf '%s' "$payload" | curl --noproxy '*' -sS \
		-H "Authorization: Bearer ${token}" \
		-H "Content-Type: application/json" \
		--data-binary @- \
		"${endpoint}") || return $?
	debug "response: $response"
	printf '%s' "$response"
}

run_list() {
	payload=$(build_payload_list) || return 1
	resp=$(mcp_post "$payload") || return $?

	if ! tools_json=$(printf '%s' "$resp" | jq -er '.result.tools' 2>/dev/null); then
		log_err "shai-remote: unexpected list response"
		log_err "$resp"
		return 1
	fi

	count=$(printf '%s' "$tools_json" | jq 'length')
	if [ "$count" -eq 0 ]; then
		printf 'no calls defined\n'
		return 0
	fi

	printf '%s' "$tools_json" | jq -r '.[] | "\(.name)\t\(.description // "")"' | while IFS="$(printf '\t')" read -r name desc; do
		if [ -n "$desc" ]; then
			printf '%s - %s\n' "$name" "$desc"
		else
			printf '%s\n' "$name"
		fi
	done
}

emit_content() {
	printf '%s' "$1" | jq -c '.result.content[]?' 2>/dev/null | while IFS= read -r chunk; do
		role=$(printf '%s' "$chunk" | jq -r '.role // "stdout"' 2>/dev/null || printf 'stdout')
		if [ "$role" = "stderr" ]; then
			printf '%s' "$chunk" | jq -j '.text // ""' >&2 2>/dev/null
		else
			printf '%s' "$chunk" | jq -j '.text // ""' 2>/dev/null
		fi
	done
}

extract_exit_code() {
	code=$(printf '%s' "$1" | jq -r '
		if .result.exitCode? then .result.exitCode
		elif .result.success? == true then 0
		else empty end' 2>/dev/null || true)
	if [ -z "${code}" ] || [ "${code}" = "null" ]; then
		code=1
	fi
	printf '%s' "$code"
}

run_call() {
	call_name=$1
	shift
	payload=$(build_payload_call "$call_name" "$@") || return 1
	resp=$(mcp_post "$payload") || return $?

	if printf '%s' "$resp" | jq -e '.error' >/dev/null 2>&1; then
		msg=$(printf '%s' "$resp" | jq -r '.error.message // "call failed"' 2>/dev/null || printf 'call failed')
		log_err "shai-remote: $msg"
		return 1
	fi

	if ! printf '%s' "$resp" | jq -e '.result' >/dev/null 2>&1; then
		log_err "shai-remote: malformed response"
		log_err "$resp"
		return 1
	fi

	emit_content "$resp"
	exit_code=$(extract_exit_code "$resp")
	return "$exit_code"
}

main() {
	require_cmd curl
	require_cmd jq

	cmd=""
	call_name=""

	while [ $# -gt 0 ]; do
		arg=$1
		shift
		case "$arg" in
			--) break ;;
			--endpoint)
				[ $# -gt 0 ] || usage
				endpoint=$1
				shift
				continue
				;;
			--token)
				[ $# -gt 0 ] || usage
				token=$1
				shift
				continue
				;;
			--session)
				[ $# -gt 0 ] || usage
				session_id=$1
				shift
				continue
				;;
			--verbose)
				cli_verbose=1
				continue
				;;
			list)
				if [ -z "$cmd" ]; then
					cmd="list"
					continue
				fi
				;;
			call)
				if [ -z "$cmd" ]; then
					cmd="call"
					continue
				fi
				;;
			*)
				if [ "$cmd" = "list" ]; then
					usage
				fi
				if [ "$cmd" = "call" ] && [ -z "$call_name" ]; then
					call_name=$arg
					break
				fi
				usage
				;;
		esac
	done

	if [ "$cmd" = "call" ]; then
		if [ $# -eq 0 ] && [ -z "$call_name" ]; then
			usage
		fi
		if [ $# -gt 0 ] && [ "$1" = "--" ]; then
			shift
		fi
		if [ -z "$call_name" ]; then
			call_name=$1
			shift
		fi
	elif [ "$cmd" = "list" ]; then
		if [ $# -gt 0 ]; then
			usage
		fi
	else
		usage
	fi

	if [ "$cli_verbose" -eq 1 ]; then
		debug_enabled=1
	fi

	ensure_env
	if [ "$cmd" = "list" ]; then
		run_list
		exit $?
	fi
	run_call "$call_name" "$@"
	exit $?
}

main "$@"
