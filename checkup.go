package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/sbeliakou/check-up/modules/bash"
	"github.com/sbeliakou/check-up/modules/helper"
	"github.com/sbeliakou/check-up/modules/jUnit"
)

var version string = "v0.2.5"
var workdir string = ""

func print(msg string) {
	if os.Getenv("TERM") == "" {
		mod := regexp.MustCompile(`\033[^m]*m`).ReplaceAllString(msg, "")
		mod = regexp.MustCompile(`✓`).ReplaceAllString(mod, "success")
		mod = regexp.MustCompile(`✗`).ReplaceAllString(mod, "FAILURE")
		log.Println(mod)
	} else {
		log.Println(msg)
	}

}

type LoopConfig struct {
	Items   []string `yaml:"items"`
	Command string   `yaml:"command"`
}

type ScenarioItem struct {
	// YAML-Defined data
	Name        string            `yaml:"name"`
	Case        string            `yaml:"case"`
	Env         map[string]string `yaml:"env"`
	Workdir     string            `yaml:"workdir"`
	Description string            `yaml:"description"`
	Script      string            `yaml:"script"`
	Skip        bool              `yaml:"skip"`
	Output      bool              `yaml:"output"`
	Weight      int               `yaml:"weight"`
	Log         string            `yaml:"log"`
	Fatal       bool              `yaml:"fatal"`
	Before      []string          `yaml:"before"`
	After       []string          `yaml:"after"`
	Loop        LoopConfig        `yaml:"loop"`
	Timeout     int               `yaml:"timeout"`

	Debug struct {
		Script  string `yaml:"script"`
		Timeout int    `yaml:"timeout"`
		stdout  string
		result  error
	} `yaml:"debug"`

	// Runtime data
	status               string
	result               error
	stdout               string
	durationString       string
	durationMilliSeconds int

	canShow bool
	canRun  bool

	env []string

	skipReason string
}

func (s *ScenarioItem) IsSuccessful() bool {
	return s.status == "success"
}

func (s *ScenarioItem) IsFailed() bool {
	return s.status == "failed"
}

func (s *ScenarioItem) CanShow() bool {
	// return s.Case != ""
	return s.canShow
}

func (s *ScenarioItem) RunBash(GlobalEnv map[string]string) ([]byte, error) {
	getIfItsGlobalEnvVar := func(envItemName string, envItemValue string) string {
		re, _ := regexp.Compile(`^\{GLOBAL:(.*)\}$`)
		if re.MatchString(envItemValue) {
			matches := re.FindStringSubmatch(envItemValue)
			if len(matches) > 1 {
				for _, v := range os.Environ() {
					key := strings.Split(v, "=")[0]
					value := strings.Split(v, "=")[1]
					if key == matches[1] {
						return fmt.Sprintf("%s=%s", envItemName, value)
					}
				}
				return fmt.Sprintf("%s=", envItemName)
			}
		}
		return fmt.Sprintf("%s=%s", envItemName, envItemValue)
	}

	var env []string
	for key, value := range GlobalEnv {
		env = append(env,
			getIfItsGlobalEnvVar(key, value),
		)
	}

	for key, value := range s.Env {
		env = append(env,
			getIfItsGlobalEnvVar(key, value),
		)
	}

	s.env = env

	stdout, err := bash.RunBashScript(s.Script, workdir, s.Timeout, s.env)
	s.stdout = strings.TrimSpace(string(stdout))
	s.result = err

	if err == nil {
		s.status = "success"
	} else {
		s.status = "failed"

		if s.Debug.Script != "" {
			debugStdout, debugErr := bash.RunBashScript(s.Debug.Script, workdir, s.Debug.Timeout, s.env)
			s.Debug.stdout = strings.TrimSpace(string(debugStdout))
			s.Debug.result = debugErr
		}
	}

	return stdout, err
}

type suitConfig struct {
	Name        string            `yaml:"name"`
	FileName    string            `yaml:"filename"`
	Cases       []ScenarioItem    `yaml:"cases"`
	CustomIndex string            `yaml:"custom_index"`
	Env         map[string]string `yaml:"env"`

	startTime time.Time
	endTime   time.Time

	all                  int
	successfull          int
	skipped              int
	failed               int
	score                float64
	durationString       string
	durationMilliSeconds int
}

func (c *suitConfig) getScenarioIds() []int {
	result := []int{}

	for i := 0; i < len(c.Cases); i++ {
		// if c.Cases[i].Skip {
		// 	continue
		// }

		if c.Cases[i].canShow || c.Cases[i].canRun {
			result = append(result, i)
		}
	}

	return result
}

func (c *suitConfig) getScenarioCount() int {
	result := 0
	for _, i := range c.getScenarioIds() {
		if c.Cases[i].CanShow() {
			result++
		}
	}

	return result
}

func (c *suitConfig) getIdByName(name string) int {
	for id, item := range c.Cases {
		if item.Name == name {
			return id
		}
	}
	return -1
}

func (c *suitConfig) printHeader() {
	scenariosCount := c.getScenarioCount()
	filename := ""
	if c.FileName != "" {
		filename = fmt.Sprintf(", file: %s", c.FileName)
	}

	c.startTime = time.Now()

	switch {
	case scenariosCount > 1:
		log.Printf("[ %s ], 1..%d tests%s\n", c.Name, scenariosCount, filename)
	case scenariosCount == 1:
		log.Printf("[ %s ], 1 test%s\n", c.Name, filename)
	case scenariosCount == 0:
		log.Printf("[ %s ], no tests to run%s\n", c.Name, filename)
	}
}

func (c *suitConfig) signOff() {
	c.endTime = time.Now()

	sum := 0
	max := 0
	skipped := 0
	failed := 0
	all := 0

	for _, i := range c.getScenarioIds() {
		item := c.Cases[i]
		if item.CanShow() {
			all++
			if item.Skip {
				skipped++
			} else {
				max += item.Weight
				if item.IsSuccessful() {
					sum += item.Weight
				} else {
					failed++
				}
			}
		}
	}

	c.successfull = all - skipped - failed
	c.skipped = skipped
	c.failed = failed
	c.all = all
	c.score = 100 * float64(sum) / float64(max)
	c.durationString, c.durationMilliSeconds = duration(c.startTime, c.endTime)
}

func (c *suitConfig) printSummary() {
	if c.all > 0 {
		failed := fmt.Sprintf("%d tests failed", c.failed)
		if c.failed > 0 {
			failed = fmt.Sprintf("\033[31m%s\033[0m", failed)
		}

		skipped := fmt.Sprintf("%d tests skipped", c.skipped)
		if c.skipped > 0 {
			skipped = fmt.Sprintf("\033[36m%s\033[0m", skipped)
		}

		if c.failed > 0 {
			print(fmt.Sprintf("%d (of %d) tests passed, %s, %s, rated as %.2f%%, spent %s", c.successfull, c.all, failed, skipped, c.score, c.durationString))
		} else {
			print(fmt.Sprintf("\033[32m%d (of %d) tests passed, %s, %s, rated as %.2f%%, spent %s\033[0m", c.successfull, c.all, failed, skipped, c.score, c.durationString))
		}
	}

	print("")
}

type taskScriptDetails struct {
	Name    string
	Script  string
	Stdout  string
	Result  error
	Timeout int
	Env     []string
}

func printOut(b string, t []taskScriptDetails, indent ...int) {
	indentStr := "     "
	log.Println(indentStr[2:] + b)

	if len(indent) > 0 {
		indentStr = indentStr + strings.Repeat(" ", indent[0])
	}

	for _, item := range t {
		if item.Name != "" {
			log.Printf(indentStr[2:]+"%s", item.Name)
		}

		log.Println(indentStr + "script: >\n  " + indentStr + regexp.MustCompile(`\n`).ReplaceAllString(item.Script, "\n  "+indentStr))

		if len(item.Stdout) == 0 {
			log.Println(indentStr + "stdout: \"\" (output is empty)")
		} else {
			log.Println(indentStr + "stdout: >\n  " + indentStr + regexp.MustCompile(`\n`).ReplaceAllString(item.Stdout, "\n  "+indentStr))
		}

		if item.Timeout != 0 {
			log.Printf(indentStr+"timeout: %d sec", item.Timeout)
		}

		exitCodeInt := 0
		if item.Result != nil {
			exitCodeInt, _ = strconv.Atoi(regexp.MustCompile(`exit status (\d+)`).FindStringSubmatch(item.Result.Error())[1])
		}

		color := "\033[31m"
		if exitCodeInt == 0 {
			color = "\033[32m"
		}
		log.Printf(indentStr+"exit code: %d (%s%s\033[0m)", exitCodeInt, color, bash.ExplainExitCode(exitCodeInt))

		if len(item.Env) > 0 {
			log.Println(indentStr + "environment:")
			for _, v := range item.Env {
				log.Println(indentStr + "  " + v)
			}
		}

		log.Println()
	}

	if len(t) == 0 {
		log.Println()
	}
}

func (c *suitConfig) printTestStatus(id int, asId ...int) {
	testCase := c.Cases[id]

	status, color := "✗", "\033[31m" // Assume failure

	if testCase.IsSuccessful() {
		status, color = "✓", "\033[32m"
	}

	if testCase.Skip {
		status, color = "-", "\033[36m"
	}

	i := id
	if len(asId) > 0 {
		i = asId[0]
	}

	caseStatusMsg := ""
	if c.CustomIndex != "" {
		dataFuncMap := template.FuncMap{
			"add": func(x, y int) int { return x + y },
		}

		data := map[string]interface{}{
			"TaskId":    i - 1,
			"TaskCount": c.getScenarioCount(),
		}

		tmpl, err := template.New("custom_index").Funcs(dataFuncMap).Parse(c.CustomIndex)
		if err != nil {
			panic(err)
		}

		var buf bytes.Buffer
		err = tmpl.Execute(&buf, data)
		if err != nil {
			panic(err)
		}
		caseStatusMsg = fmt.Sprintf("%s %s", buf.String(), testCase.Case)
	} else {
		if testCase.Case != "" {
			caseStatusMsg = fmt.Sprintf("%2d/%d  %s", i, c.getScenarioCount(), testCase.Case)
		} else {
			caseStatusMsg = fmt.Sprintf("%2s/%s  %s", "-", "-", "Silent task, not scored")
		}
	}

	if testCase.Skip {
		caseStatusMsg = fmt.Sprintf("%s%s %s, skipping reason: %s \033[0m", color, status, caseStatusMsg, testCase.skipReason)
	} else {
		caseStatusMsg = fmt.Sprintf("%s%s %s, %s\033[0m", color, status, caseStatusMsg, testCase.durationString)
	}

	for _, j := range c.getScenarioIds() {
		if j == id {
			if testCase.CanShow() || (verbosity >= 3) {
				log.Print(caseStatusMsg)

				if testCase.Skip {
					return
				}

				if len(testCase.Before) > 0 && (verbosity == 3 || verbosity == 4) {
					beforeScripts := []taskScriptDetails{}
					l := len(testCase.Before)
					for i, name := range testCase.Before {
						beforeScripts = append(beforeScripts, taskScriptDetails{
							Name:    fmt.Sprintf("%d/%d: %s", i+1, l, strings.TrimSpace(c.Cases[c.getIdByName(name)].Name)),
							Script:  strings.TrimSpace(c.Cases[c.getIdByName(name)].Script),
							Stdout:  strings.TrimSpace(c.Cases[c.getIdByName(name)].stdout),
							Result:  c.Cases[c.getIdByName(name)].result,
							Timeout: c.Cases[c.getIdByName(name)].Timeout,
						})
					}
					printOut(fmt.Sprintf("pre-tasks (%d):", l), beforeScripts, 2)
				}

				if (verbosity == 1 && testCase.IsFailed()) ||
					(verbosity == 2) ||
					(verbosity == 3) ||
					(verbosity == 4) {

					if (verbosity == 4) && testCase.IsFailed() {
						printOut("main script:", []taskScriptDetails{
							{
								Script:  strings.TrimSpace(testCase.Script),
								Stdout:  strings.TrimSpace(testCase.stdout),
								Result:  testCase.result,
								Timeout: testCase.Timeout,
								Env:     testCase.env,
							},
						})
					} else {
						printOut("main script:", []taskScriptDetails{
							{
								Script:  strings.TrimSpace(testCase.Script),
								Stdout:  strings.TrimSpace(testCase.stdout),
								Result:  testCase.result,
								Timeout: testCase.Timeout,
							},
						})
					}

					if testCase.IsFailed() && (verbosity == 3 || verbosity == 4) {
						if len(strings.TrimSpace(testCase.Debug.Script)) > 0 {
							printOut("debug:", []taskScriptDetails{
								{
									Script:  strings.TrimSpace(testCase.Debug.Script),
									Stdout:  strings.TrimSpace(testCase.Debug.stdout),
									Result:  testCase.Debug.result,
									Timeout: testCase.Debug.Timeout,
								},
							})
						} else {
							printOut("debug: script undefined", []taskScriptDetails{})
						}
					}

					if len(testCase.After) > 0 && (verbosity == 3 || verbosity == 4) {
						afterScript := []taskScriptDetails{}
						l := len(testCase.After)
						for i, name := range testCase.After {
							afterScript = append(afterScript, taskScriptDetails{
								Name:    fmt.Sprintf("%d/%d: %s", i+1, l, strings.TrimSpace(c.Cases[c.getIdByName(name)].Name)),
								Script:  strings.TrimSpace(c.Cases[c.getIdByName(name)].Script),
								Stdout:  strings.TrimSpace(c.Cases[c.getIdByName(name)].stdout),
								Result:  c.Cases[c.getIdByName(name)].result,
								Timeout: c.Cases[c.getIdByName(name)].Timeout,
							})
						}
						printOut(fmt.Sprintf("post-tasks (%d):", l), afterScript, 2)
					}
				}
			}
			return
		}
	}
}

func (c *suitConfig) execTask(item int) {
	testCase := &c.Cases[item]

	if testCase.Skip {
		testCase.skipReason = "'skip=true' setting"
		return
	}

	if testCase.Script == "" {
		testCase.skipReason = "empty 'script' setting"
		testCase.Skip = true
		return
	}

	taskStartTime := time.Now()

	for _, name := range testCase.Before {
		c.Cases[c.getIdByName(name)].RunBash(c.Env)
	}

	testCase.RunBash(c.Env)

	for _, name := range testCase.After {
		c.Cases[c.getIdByName(name)].RunBash(c.Env)
	}

	testCase.durationString, testCase.durationMilliSeconds = duration(taskStartTime, time.Now())
}

func (t *suitConfig) getConf(config string, taskFilter ...string) *suitConfig {
	yamlFile, err := os.ReadFile(config)

	if err != nil {
		log.Fatal(err)
	}

	err = yaml.Unmarshal(yamlFile, t)
	if err != nil {
		log.Fatalf("Cannot recognize configuration structure in file: %s", config)
	}

	// var envs map[string]string
	wdir, _ := os.Getwd()
	if workdir != "" {
		wdir = workdir
	}

	// if (*t).Env == nil {
	// 	(*t).Env = make(map[string]string)
	// }

	a := &suitConfig{
		Name:        (*t).Name,
		CustomIndex: (*t).CustomIndex,
		Cases:       []ScenarioItem{},
		// Env:         t.Env,
	}

	for i := 0; i < len((*t).Cases); i++ {
		if (*t).Env != nil {
			if (*t).Cases[i].Env == nil {
				(*t).Cases[i].Env = make(map[string]string)
			}
			for key, value := range (*t).Env {
				if _, exists := (*t).Cases[i].Env[key]; !exists {
					(*t).Cases[i].Env[key] = value
				}
			}
		}

		if (*t).Cases[i].Workdir != "" {
			wdir = (*t).Cases[i].Workdir
		} else {
			(*t).Cases[i].Workdir = wdir
			if wdir == "" {
				wdir, _ = os.Getwd()
			}
		}

		if (*t).Cases[i].Case != "" {
			if len(taskFilter) > 0 {
				if strings.Contains((*t).Cases[i].Case, taskFilter[0]) {
					(*t).Cases[i].canShow = true
					(*t).Cases[i].canRun = true
				}
			} else {
				(*t).Cases[i].canShow = true
				(*t).Cases[i].canRun = true
			}
		}

		if (*t).Cases[i].Name == "" {
			(*t).Cases[i].canRun = true
		}

		if (*t).Cases[i].CanShow() {
			if (*t).Cases[i].Weight == 0 {
				(*t).Cases[i].Weight = 1
			}
		}

		if *timeout > 0 {
			(*t).Cases[i].Timeout = *timeout
			(*t).Cases[i].Debug.Timeout = *timeout
		}

		if len((*t).Cases[i].Loop.Items) > 0 || len((*t).Cases[i].Loop.Command) > 0 {
			Items := []string{}

			if len((*t).Cases[i].Loop.Items) > 0 {
				Items = (*t).Cases[i].Loop.Items
			}

			if len((*t).Cases[i].Loop.Command) > 0 {
				s := (*t).Cases[i]
				s.Script = s.Loop.Command
				stdout, _ := s.RunBash((*t).Env)

				for _, item := range strings.Split(string(stdout), "\n") {
					if item != "" {
						Items = append(Items, item)
					}
				}
			}

			for _, item := range Items {
				last := len(a.Cases)
				a.Cases = append(a.Cases, t.Cases[i])

				nameHasItemVar := regexp.MustCompile(`\$\{?item\}?`)
				if len(nameHasItemVar.FindStringSubmatch((*t).Cases[i].Case)) > 0 {
					(*a).Cases[last].Case = nameHasItemVar.ReplaceAllString((*t).Cases[i].Case, item)
				} else {
					(*a).Cases[last].Case = fmt.Sprintf("%s, item => \"%s\"", (*t).Cases[i].Case, item)
				}

				a.Cases[last].Env = make(map[string]string)
				for k, v := range t.Cases[i].Env {
					a.Cases[last].Env[k] = v
				}

				a.Cases[last].Env["item"] = item
			}

		} else {
			a.Cases = append(a.Cases, t.Cases[i])
		}

	}
	t = a
	return t
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

func duration(start time.Time, finish time.Time) (string, int) {
	result := finish.Sub(start).Truncate(time.Millisecond)
	resultInMilliSeconds := int(result.Milliseconds())
	return result.String(), resultInMilliSeconds
}

type reportFile struct {
	fileName string
	format   string
}

func (r *reportFile) parse(d string) {
	config := strings.Split(d, "=")
	if len(config) > 1 {
		(*r).fileName = config[1]
		(*r).format = config[0]
	}
}

var report reportFile

func jUnitReportSave(reportFile string, c suitConfig) {
	if reportFile != "" {

		T := struct {
			SuitName    string
			TotalTests  int
			FailedTests int
			Tests       []ScenarioItem
			TotalTime   string
			TimeStamp   string
			Verbosity   int
		}{
			SuitName:    c.Name,
			TotalTests:  c.all,
			FailedTests: c.failed,
			Tests:       c.Cases,
			TotalTime:   c.durationString,
			TimeStamp:   time.Now().Format("2006-01-02T15:04:05"),
			Verbosity:   0,
		}

		funcMap := template.FuncMap{
			"Quote": func(m string) string {
				return strconv.Quote(m)
			},
		}

		reportFile, err := os.Create(reportFile)
		if err != nil {
			log.Println(err)
			return
		}
		defer reportFile.Close()

		jut, _ := template.New("junit report").Funcs(funcMap).Parse(string(jUnit.JUnitTemplate))
		jut.Execute(reportFile, T)
	}
}

func jsonReportSave(reportFile string, c suitConfig) {
	type TestData struct {
		Name     string `json:"name"`
		Status   bool   `json:"status"`
		Duration string `json:"duration"`
		Stdout   string `json:"stdout"`
	}

	type TestsSummary struct {
		Success  int     `json:"success"`
		Failed   int     `json:"failed"`
		Rating   float64 `json:"rating"`
		Duration string  `json:"duration"`
	}

	type JsonStructure struct {
		TestName string       `json:"testName"`
		Tests    []TestData   `json:"tests"`
		Summary  TestsSummary `json:"summary"`
	}

	var jsonReportData JsonStructure
	jsonReportData.TestName = c.Name
	jsonReportData.Tests = []TestData{}

	if c.getScenarioCount() > 0 {
		for _, id := range c.getScenarioIds() {
			if c.Cases[id].CanShow() {
				t := TestData{
					Name:     c.Cases[id].Case,
					Status:   c.Cases[id].IsSuccessful(),
					Duration: c.Cases[id].durationString,
				}

				if (verbosity > 1 && c.Cases[id].IsFailed()) || (verbosity > 2) {
					t.Stdout = c.Cases[id].stdout
				}

				jsonReportData.Tests = append(jsonReportData.Tests, t)
			}
		}
	}

	jsonReportData.Summary = TestsSummary{
		Success:  c.successfull,
		Failed:   c.failed,
		Rating:   c.score,
		Duration: c.durationString,
	}

	reportJson, _ := json.MarshalIndent(jsonReportData, "", "  ")
	os.WriteFile(reportFile, reportJson, 0644)
}

var (
	localConfig               = flag.String("c", "", "Local tests case file path (Required unless -C cpecified)")
	remoteConfig              = flag.String("C", "", "Remote tests case file url (Required unless -c specified)")
	filter                    = flag.String("f", "", "Run tests by name regexp match")
	wdir                      = flag.String("w", "", "Set working Dir")
	reportFlag                = flag.String("o", "", "JSON or JUnit report file")
	timeout                   = flag.Int("t", 0, "Timeout of the task execution")
	generateSampleTesCaseFile = flag.Bool("g", false, "")
)

var verbosity int = 0

func listFiles(path string) []string {
	var result []string

	var walkDir func(dirPath string)
	walkDir = func(dirPath string) {
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			log.Fatal(err)
		}

		for _, entry := range entries {
			fullPath := filepath.Join(dirPath, entry.Name())
			if entry.IsDir() {
				walkDir(fullPath)
			} else if filepath.Ext(fullPath) == ".yaml" || filepath.Ext(fullPath) == ".yml" {
				result = append(result, fullPath)
			}
		}
	}

	f, err := os.Stat(path)
	if err != nil {
		log.Fatal(err)
	}

	if f.IsDir() {
		walkDir(path)
	} else {
		result = append(result, path)
	}

	if len(result) == 0 {
		log.Fatalf("There are no yaml or yml files found in the path: %s", path)
	}

	return result
}

func main() {
	log.SetFlags(0)

	// Modified Args slice
	args := os.Args[:1] // keep the program name
	verbosity = 0
	for _, arg := range os.Args[1:] {
		if arg != "--version" {
			matches := regexp.MustCompile(`^-v(v+)?=?(\d+)?$|^--verbosity=(\d+)$`).FindStringSubmatch(arg)
			if len(matches) > 0 {
				if matches[0] == "-v" && matches[1] == "" { // Handle -v, means the same as -v=1
					verbosity = 1
				} else if matches[1] != "" { // Handle -vv, -vvv (matches multiple 'v' after the first)
					verbosity = len(matches[1]) + 1 // +1 because the first 'v' wasn't counted in matches[1]
				} else if matches[2] != "" { // Handle -v=2, -v=3, etc.
					verbosity, _ = strconv.Atoi(matches[2])
				} else if matches[3] != "" { // Handle --verbosity=2, --verbosity=3, etc.
					verbosity, _ = strconv.Atoi(matches[3])
				}

			} else {
				args = append(args, arg) // include other args to be parsed by flag package
			}
		} else {
			log.Println(version)
			os.Exit(0)
		}
	}

	os.Args = args

	flag.Usage = helper.CustomUsage
	flag.Parse()

	if *generateSampleTesCaseFile {
		log.Printf(helper.SampleTestFile)
		os.Exit(0)
	}

	workdir = *wdir
	report.parse(*reportFlag)

	d := []suitConfig{}
	if *localConfig != "" {
		for _, file := range listFiles(*localConfig) {
			if len(file) > 0 {
				d = append(d, *(&suitConfig{}).getConf(file, *filter))
				cwdir, _ := os.Getwd()
				d[len(d)-1].FileName = strings.Replace(file, cwdir, ".", 1)
			}
		}
	}

	if *remoteConfig != "" {
		tmpDir, err := os.MkdirTemp("/var/tmp", ".")
		if err != nil {
			log.Fatalf("Failed to create a temporary directory: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		tmpFile, err := os.CreateTemp(tmpDir, "tmp.*")
		if err != nil {
			log.Fatalf("Failed to create a temporary file: %v", err)
		}
		defer tmpFile.Close()

		if matched, _ := regexp.MatchString("^http(s)?://", *remoteConfig); matched {
			load(tmpFile, *remoteConfig)
			*localConfig = tmpFile.Name()
		}

		d = append(d, *(&suitConfig{}).getConf(*localConfig, *filter))
	}

	if len(d) > 0 {
		for _, c := range d {
			handleScenarios(&c)
			handleReports(&c)
		}
	} else {
		flag.Usage()
	}
}

func handleScenarios(c *suitConfig) {
	c.printHeader()
	if c.getScenarioCount() > 0 {
		max := 30
		for i, id := range c.getScenarioIds() {
			taskTitle := fmt.Sprintf("   %d/%d  %s", i, c.getScenarioCount(), c.Cases[id].Case)
			if max < len(taskTitle) {
				max = len(taskTitle)
			}
		}

		log.Println(strings.Repeat("-", max+7))
		j := 0
		for _, id := range c.getScenarioIds() {
			c.execTask(id)
			if c.Cases[id].CanShow() {
				c.printTestStatus(id, j+1)
				j++
			} else if c.Cases[id].Case == "" {
				c.printTestStatus(id, j+1)
			}
		}

		log.Println(strings.Repeat("-", max+7))
	}
	c.signOff()
	c.printSummary()
}

func handleReports(c *suitConfig) {
	switch report.format {
	case "junit":
		jUnitReportSave(report.fileName, *c)
	case "json":
		jsonReportSave(report.fileName, *c)
	}
}
