package bash

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"text/template"
)

const bashScript = `#!/usr/bin/env bash
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

if [ ${rc:=$?} -ne 0 ]; then
  exit $rc
else 
  exit ${status:-0}
fi
`

func RunBashScript(command string, workdir string, timeout int, env []string) ([]byte, error) {
	var stdout []byte = []byte("")
	var err error = nil

	if command != "" {
		tmpDir, _ := os.MkdirTemp("/var/tmp", "._")
		defer os.RemoveAll(tmpDir)

		tmpFile, _ := os.CreateTemp(tmpDir, "tmp.*")

		T := struct {
			Script string
		}{
			Script: command,
		}

		tmpl, _ := template.New("bash-script").Parse(string(bashScript))
		tmpl.Execute(tmpFile, T)

		script := exec.Command("timeout", strconv.Itoa(timeout), "/bin/bash", tmpFile.Name())
		script.Dir = workdir
		script.Env = env

		stdout, err = script.CombinedOutput()

		re, _ := regexp.Compile(fmt.Sprintf("%s: line [\\d]+: ", tmpFile.Name()))
		stdout = []byte(re.ReplaceAllString(string(stdout), ""))

		return stdout, err
	}

	return []byte(""), nil
}

func ExplainExitCode(code int) string {
	switch code {
	case 0:
		return "Script finished successfully"
	case 1:
		return "General error"
	case 2:
		return "Misuse of shell builtins, Incorrect usage of a shell built-in"
	case 124:
		return "General error, also may be Script terminated by Timeout"
	case 126:
		return "Script invoked cannot execute, Permission problem or not an executable"
	case 127:
		return "Script not found, Possible typo or the command can't be found in PATH"
	case 128:
		return "Invalid argument to exit, Exit with integer args in the range 0-255"
	case 130:
		return "Script terminated by Control-C, Terminated by the user via SIGINT (Ctrl+C)"
	case 255:
		return "Exit status out of range, Specified exit status is out of the expected range"
	default:
		if code >= 128 && code <= 255 {
			signalNumber := code - 128
			return fmt.Sprintf("Fatal error signal '%d' - Process received a critical signal and has stopped", signalNumber)
		}
		return "General error"
	}
}
