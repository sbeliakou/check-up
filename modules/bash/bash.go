package bash

const BashScript = `#!/usr/bin/env bash
set -e

output=""
status=0
lines=()

function fail() {
  echo $@
  exit 1
}

function ok() {
  echo -n
	status=0
}

function skip() {
  [ -n $@ ] && echo $@
  echo "SKIPPED DUE TO SCRIPT DECISION ..."
  exit
}

function run() {
  local origFlags="$-"
  set +eET
  local origIFS="$IFS"
  # 'output', 'status' are global variables available to tests.
  # shellcheck disable=SC2034
  # output="$("$@" 2>&1)"
  # output="$(bash -c "$@" 2>&1)"

  # shellcheck disable=SC2034
  #status="$?"

  outfile=$(mktemp)
  # echo "$@" | bash 2>&1 > $outfile
	cmd=""
	for var in "$@"; do
			if [[ "$var" =~ ( |\||\&|;) ]]; then
				cmd="$cmd \"$var\""
			else
				cmd="$cmd $var"
			fi
	done
	
  if [ $# -eq 1 ]; then
  	echo "bash -c $cmd" | bash 2>&1 > $outfile
		status="$?"
	else
	  echo "${cmd}" | bash 2>&1 > $outfile
	  status="$?"
	fi

  output="$(cat ${outfile})"
	
  # shellcheck disable=SC2034,SC2206
  IFS=$'\n' lines=($output)
  IFS="$origIFS"
  set "-$origFlags"

	echo "(run, $(pwd)) => ${cmd}"
	# [ -n "${output}" ] && echo "output=${output}"
	echo "rc: ${status}"
	if [ ${#lines[@]} -gt 1 ]; then
		echo "output: |"
		echo "$output" | sed 's/^/  /'
	else
		echo "output: '${output}'"
	fi
	
  return 0
}

function assert_success() {
  [ $# -gt 0 ] && run "$@"
	[ ${status:-0} -ne 0 ] && fail "CMD Failed: $@" || ok
}

function assert_failure() {
  [ ${status:-0} -eq 0 ] && fail "RC Assertion Failed: ${status} == 0, but shouldn't be 0" || ok
}

function assert_equal() {
  [[ "x$1" != "x$2" ]] && fail "Assertion Failed: '$1' != '$2'" || ok
}

function assert_not_equal() {
  [[ "x$1" == "x$2" ]] && fail "Assertion Failed: '$1' == '$2'" || ok
}

function assert_output() {
  case "$1" in
  -p|--partial) 
    [[ "$output" =~ "$2" ]] && ok || fail "Stdout Assertion Failed (Partial, '$2')"
    ;;
  -e|--regexp)
    echo "$output" | grep -E "$2" >/dev/null && ok || fail "Stdout Assertion Failed (Regexp, '$2')"
    ;;
  *) 
    [ "$output" == "$@" ] && ok || fail "Stdout Assertion Failed (Full mistmatch)"
    ;;
  esac
}

{{ .Script }}
exit ${status:-0}
`