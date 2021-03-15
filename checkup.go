package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v2"
)

type testCases []struct {
	Case        string            `yaml:"case"`
	GlobalEnv   map[string]string `yaml:"global_env"`
	Workdir     string            `yaml:"workdir"`
	Description string            `yaml:"description"`
	Script      string            `yaml:"script"`
	Skip        bool              `yaml:"skip"`
	Output      bool              `yaml:"output"`
	Weight      int               `yaml:"weight"`
	Log         string            `yaml:"log"`
	Fatal       bool              `yaml:"fatal"`
	Debug       string            `yaml:"debug"`
}

type testConfig struct {
	Name  string    `yaml:"name"`
	Cases testCases `yaml:"cases"`
}

type stats struct {
	TestName   string
	TestStatus string
	TestOutput string
	TestTime   string
}

const runScript = `#!/usr/bin/env bash
set -e

[ "$0" != "/tmp/script.sh" ] && cp $0 /tmp/script.sh

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
	echo 'here'
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

const jUnitTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites time="">
	<testsuite name="{{ .SuitName }}" tests="{{ .TotalTests }}" failures="{{ .FailedTests }}" errors="0" skipped="0" time="{{ .TotalTime }}" timestamp="2020-10-25T13:53:31" hostname="38b98cdc4272">

	{{ $verbosity := .Verbosity }}
	{{- range $t := .Tests }}
	{{- if eq $t.TestStatus "success" }}
		<testcase classname="{{ $.SuitName }}" name={{ $t.TestName }} time="{{ $t.TestTime }}">
			<!-- system-out>STDOUT text</system-out -->
		</testcase>
	{{- else }}
		<testcase classname="{{ $.SuitName }}" name={{ $t.TestName }} time="{{ $t.TestTime }}">
			{{ if gt $verbosity 1 }}<failure type="failure">{{ $t.TestOutput }}</failure>{{ end }}
		</testcase>
	{{- end}}
  {{- end }}
  </testsuite>
</testsuites>
`

var verbosity int = 0
var workdir string = ""
var suitName string = ""

func (t *testConfig) getConf(config string) *testConfig {
	yamlFile, err := ioutil.ReadFile(config)

	if err != nil {
		log.Fatal(err)
	}

	err = yaml.Unmarshal(yamlFile, t)
	if err != nil {
		log.Fatal(fmt.Sprintf("Cannot recognize configuration structure in %s file: ", config))
	}

	var envs map[string]string
	wdir, _ := os.Getwd()
	if workdir != "" {
		wdir = workdir
	}

	for i := 0; i < len((*t).Cases); i++ {
		if (*t).Cases[i].GlobalEnv != nil {
			envs = (*t).Cases[i].GlobalEnv
		} else {
			(*t).Cases[i].GlobalEnv = envs
		}

		if (*t).Cases[i].Workdir != "" {
			wdir = (*t).Cases[i].Workdir
		} else {
			(*t).Cases[i].Workdir = wdir
			if wdir == "" {
				wdir, _ = os.Getwd()
			}
		}

		if (*t).Cases[i].Log != "" {
			logging := (*t).Cases[i].Log
			if logging == "True" || logging == "true" || logging == "Yes" || logging == "yes" {
				(*t).Cases[i].Log = "true"
			}
		}
	}
	return t
}

func run(content string, workdir string, envs map[string]string, debug string) ([]byte, error) {
	if verbosity > 3 {
		log.Print("script content: ", content)
	}
	if content != "" {
		tmpDir, _ := ioutil.TempDir("/var/tmp", "._")
		if verbosity > 4 {
			defer os.RemoveAll(tmpDir)
		}

		tmpFile, _ := ioutil.TempFile(tmpDir, "tmp.*")
		if verbosity > 3 {
			log.Printf("run(): %s", tmpFile.Name())
		}

		T := struct {
			Script string
			Debug  string
		}{
			Script: content,
			Debug:  debug,
		}

		tmpl, _ := template.New("script").Parse(string(runScript))
		tmpl.Execute(tmpFile, T)

		script := exec.Command("/bin/bash", tmpFile.Name())
		script.Dir = workdir
		script.Env = os.Environ()
		for key, value := range envs {
			script.Env = append(script.Env,
				fmt.Sprintf("%s=%s", key, value),
			)
		}

		return script.CombinedOutput()
	}
	return []byte(""), nil
}

func load(tmpFile *os.File, URL string) error {
	resp, err := http.Get(URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(tmpFile, resp.Body)
	return err
}

func duration(start time.Time, finish time.Time) string {
	return finish.Sub(start).Truncate(time.Millisecond).String()
}

func main() {
	config := flag.String("c", "", "Local config path")
	remoteConfig := flag.String("C", "", "Remote config url")

	filter := flag.String("f", "", "Run tests by regexp match")
	wdir := flag.String("w", "", "Set working Dir")

	juReportFile := flag.String("j", "", "JUnit report file")

	// -o junit=...
	// -o json=...

	// -v1   show description if it's set
	// -v2  show failed outputs
	// -v3 show failed and successful outputs
	v1 := flag.Bool("v1", false, "Verbosity Mode 1")
	v2 := flag.Bool("v2", false, "Verbosity Mode 2")
	v3 := flag.Bool("v3", false, "Verbosity Mode 2")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Custom help %s:\n", os.Args[0])

		flag.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(os.Stderr, "    %v\n", f.Usage) // f.Name, f.Value
		})
	}

	flag.Parse()

	verbosity := func() int {
		if *v1 {
			return 1
		}
		if *v2 {
			return 2
		}
		if *v3 {
			return 3
		}
		return 0
	}()

	workdir = *wdir

	tmpDir, _ := ioutil.TempDir("/var/tmp", ".")
	defer os.RemoveAll(tmpDir)
	tmpFile, _ := ioutil.TempFile(tmpDir, "tmp.*")
	tmpFileName := tmpFile.Name()

	if *config == "" && *remoteConfig == "" {
		log.Fatal("Please specify -c or -C")
	}

	if len(regexp.MustCompile("^http(s)?:").FindStringSubmatch(*config)) > 0 {
		load(tmpFile, *config)
		config = &tmpFileName
		log.Println("tmpFile:", tmpFile.Name())
	}

	if verbosity > 1 {
		log.Println("config:", *config)
		log.Println("verbosity:", verbosity)
	}

	var c testConfig
	c.getConf(*config)

	suitName = c.Name

	total := 0
	tests := []int{}

	for i := 0; i < len(c.Cases); i++ {
		if c.Cases[i].Skip {
			continue
		}

		if *filter != "" {
			if strings.Contains(c.Cases[i].Case, *filter) {
				tests = append(tests, i)
				if c.Cases[i].Case != "" {
					total++
				}

			}
		} else {
			tests = append(tests, i)
			if c.Cases[i].Case != "" {
				total++
			}
		}
	}

	gainedWeights := 0
	totalWeights := 0

	success := 0
	failed := 0

	if total > 0 {
		log.Println("-----------------------------------------------------------------------------------")
		if suitName != "" {
			if total == 1 {
				log.Printf("Running '%s', 1 test\n", suitName)
			} else {
				log.Printf("Running '%s', 1..%d tests\n", suitName, total)
			}
		} else {
			if total == 1 {
				log.Printf("Running 1 test")
			} else {
				log.Printf("Running 1..%d tests\n", total)
			}
		}
		log.Println("-----------------------------------------------------------------------------------")
	}

	stat := []stats{}

	startTime := time.Now()

	taskStartTime := time.Now()
	taskFinishTime := time.Now()

	for i := 0; i < len(tests); i++ {
		test := c.Cases[tests[i]]

		if test.Case != "" {
			if test.Weight == 0 {
				test.Weight = 1
			}

			totalWeights = totalWeights + test.Weight

			if verbosity > 3 {
				log.Println()
				log.Printf("Running - %s\n", test.Case)
			}
		}

		taskStartTime = time.Now()
		stdout, err := run(test.Script, test.Workdir, test.GlobalEnv, test.Debug)
		taskFinishTime = time.Now()

		if test.Case != "" {
			if err == nil {
				if suitName != "" {
					// log.Printf("\033[32m✓ [%s] => %s (%d), %s\033[0m\n", suitName, test.Case, test.Weight, taskFinishTime.Sub(taskStartTime).Truncate(time.Millisecond).String())
					log.Printf("\033[32m✓ [%s] => %s (%d), %s\033[0m\n", suitName, test.Case, test.Weight, duration(taskStartTime, taskFinishTime))
				} else {
					log.Printf("\033[32m✓ %s (%d), %s\033[0m\n", test.Case, test.Weight, duration(taskStartTime, taskFinishTime))
				}

				gainedWeights = gainedWeights + test.Weight
				success++
				stat = append(stat, stats{TestName: strconv.Quote(test.Case), TestStatus: "success", TestOutput: strconv.Quote(string(stdout)), TestTime: duration(taskStartTime, taskFinishTime)})
			} else {
				if suitName != "" {
					log.Printf("\033[31m✗ [%s] -> %s\033[0m\n", suitName, test.Case)
				} else {
					log.Printf("\033[31m✗ %s\033[0m\n", test.Case)
				}

				failed++
				stat = append(stat, stats{TestName: strconv.Quote(test.Case), TestStatus: "failed", TestOutput: strconv.Quote(string(stdout)), TestTime: duration(taskStartTime, taskFinishTime)})
			}

			if verbosity > 0 && test.Description != "" {
				log.Printf("  Description: %s", test.Description)
			}

			if verbosity == 2 && err != nil {
				log.Println("  Result:", err)
				log.Print(fmt.Sprintf("  Output:\n%s", string(stdout)))
			}

			if verbosity == 3 {
				log.Println("  Result:", err)
				log.Print(fmt.Sprintf("  Output:\n%s", string(stdout)))
			}
		}
	}
	finishTime := time.Now()

	log.Println("-----------------------------------------------------------------------------------")
	log.Println("Tests Summary:")

	// msgTestPassed := fmt.Sprintf("%d (of %d) tests passed", success, total)
	// msgTestFailed := fmt.Sprintf("%d tests failed;", failed)

	// if failed > 0 {

	// } else {
	// 	msgTestPassed = fmt.Sprintf("%d (of %d) tests passed", success, total)
	// }

	if failed > 0 {
		log.Printf("  %d (of %d) tests passed, \033[31m%d tests failed;\033[0m rated as %.2f%%", success, total, failed, 100*float64(gainedWeights)/float64(totalWeights))
	} else {
		log.Printf("  \033[32m%d (of %d) tests passed,\033[0m %d tests failed; rated as %.2f%%", success, total, failed, 100*float64(gainedWeights)/float64(totalWeights))
	}

	log.Println()
	log.Println("Time Spent: ", duration(startTime, finishTime))
	log.Println("-----------------------------------------------------------------------------------")

	if *juReportFile != "" {
		T := struct {
			SuitName    string
			TotalTests  int
			FailedTests int
			Tests       []stats
			TotalTime   string
			Verbosity   int
		}{
			SuitName:    suitName,
			TotalTests:  total,
			FailedTests: failed,
			Tests:       stat,
			TotalTime:   duration(startTime, finishTime),
			Verbosity:   verbosity,
		}

		reportFile, err := os.Create(*juReportFile)
		defer reportFile.Close()
		if err != nil {
			log.Println(err)
			return
		}

		jut, _ := template.New("junit report").Parse(string(jUnitTemplate))
		jut.Execute(reportFile, T)
	}
}
