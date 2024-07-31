package main

import (
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
	"github.com/sbeliakou/check-up/modules/jUnit"
)

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
	GlobalEnv   map[string]string `yaml:"global_env"`
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

	Debug       string `yaml:"debug"`
	DebugScript string `yaml:"debug_script"`
	DebugStdout string
	DebugResult error

	// Runtime data
	Status   string
	Result   error
	Stdout   string
	Duration string

	canShow bool
	canRun  bool
}

func (s *ScenarioItem) IsSuccessful() bool {
	return s.Status == "success"
}

func (s *ScenarioItem) IsFailed() bool {
	return s.Status == "failed"
}

func (s *ScenarioItem) CanShow() bool {
	// return s.Case != ""
	return s.canShow
}

func (s *ScenarioItem) RunBash() ([]byte, error) {
	var env []string = os.Environ()
	for key, value := range s.GlobalEnv {
		env = append(env,
			fmt.Sprintf("%s=%s", key, value),
		)
	}

	for key, value := range s.Env {
		env = append(env,
			fmt.Sprintf("%s=%s", key, value),
		)
	}

	stdout, err := bash.RunBashScript(s.Script, workdir, env)
	s.Stdout = strings.TrimSpace(string(stdout))
	s.Result = err

	if err == nil {
		s.Status = "success"
	} else {
		s.Status = "failed"

		if s.DebugScript != "" {
			debug_stdout, debug_err := bash.RunBashScript(s.DebugScript, workdir, env)
			s.DebugStdout = strings.TrimSpace(string(debug_stdout))
			s.DebugResult = debug_err
		}
	}

	return stdout, err
}

type suitConfig struct {
	Name     string         `yaml:"name"`
	FileName string         `yaml:"filename"`
	Cases    []ScenarioItem `yaml:"cases"`

	startTime time.Time
	endTime   time.Time

	all         int
	successfull int
	failed      int
	score       float64
	duration    string
}

func (c *suitConfig) getScenarioIds() []int {
	result := []int{}

	for i := 0; i < len(c.Cases); i++ {
		if c.Cases[i].Skip {
			continue
		}

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
	failed := 0
	all := 0

	for _, i := range c.getScenarioIds() {
		item := c.Cases[i]
		if item.CanShow() {
			all++
			max += item.Weight
			if item.IsSuccessful() {
				sum += item.Weight
			} else {
				failed++
			}
		}
	}

	c.successfull = all - failed
	c.failed = failed
	c.all = all
	c.score = 100 * float64(sum) / float64(max)
	c.duration = duration(c.startTime, c.endTime)
}

func (c *suitConfig) printSummary() {
	if c.all > 0 {
		if c.failed > 0 {
			print(fmt.Sprintf("%d (of %d) tests passed, \033[31m%d tests failed,\033[0m rated as %.2f%%, spent %s", c.successfull, c.all, c.failed, c.score, c.duration))
		} else {
			print(fmt.Sprintf("\033[32m%d (of %d) tests passed, %d tests failed, rated as %.2f%%, spent %s\033[0m", c.successfull, c.all, c.failed, c.score, c.duration))
		}
	}
	print("")
}

func printOutTaskResultDetails(indent int, std string, text ...string) {
	indent_str1 := strings.Repeat(" ", indent)
	indent_str2 := strings.Repeat(" ", indent+2)

	if len(text) > 0 {
		txt := strings.TrimSpace(text[0])
		if txt == "" {
			log.Printf(indent_str1+"- %s: \"\" (null)\n", std)
			return
		}

		log.Printf(indent_str1+"- %s: >\n%s\n", std, regexp.MustCompile(`\n`).ReplaceAllString(indent_str2+"  "+txt, "\n  "+indent_str2))
	} else {
		log.Printf(indent_str1+"%s\n", std)
	}
}

func (c *suitConfig) printTestStatus(id int, asId ...int) {
	testCase := c.Cases[id]
	i := id
	if len(asId) > 0 {
		i = asId[0]
	}

	for _, j := range c.getScenarioIds() {
		if j == id {
			if testCase.CanShow() {
				if testCase.IsSuccessful() {
					print(fmt.Sprintf("\033[32m✓ %2d/%d  %s, %s\033[0m", i, c.getScenarioCount(), testCase.Case, testCase.Duration))
				} else {
					print(fmt.Sprintf("\033[31m✗ %2d/%d  %s, %s\033[0m", i, c.getScenarioCount(), testCase.Case, testCase.Duration))
				}

				if (verbosity > 1 && testCase.IsFailed()) || (verbosity > 2) {
					if len(testCase.Before) > 0 {
						printOutTaskResultDetails(3, "pre-tasks:")
					}

					for _, name := range testCase.Before {
						printOutTaskResultDetails(5, "- name: "+name)
						printOutTaskResultDetails(7, "script", c.Cases[c.getIdByName(name)].Script)
						printOutTaskResultDetails(7, "stdout", c.Cases[c.getIdByName(name)].Stdout)

						if c.Cases[c.getIdByName(name)].Result == nil {
							printOutTaskResultDetails(7, "- exit status: 0 (\033[32msuccess\033[0m)")
						} else {
							printOutTaskResultDetails(7, strings.Replace(fmt.Sprintf("- %s (\033[31mfail\033[0m)", c.Cases[c.getIdByName(name)].Result), "exit status", "exit status:", 1))
						}
					}
					if len(testCase.Before) > 0 {
						printOutTaskResultDetails(5, "")
					}

					printOutTaskResultDetails(3, "main task:")
					printOutTaskResultDetails(5, "script", testCase.Script)
					printOutTaskResultDetails(5, "stdout", testCase.Stdout)

					if testCase.Result == nil {
						printOutTaskResultDetails(5, "- exit status: 0 (\033[32msuccess\033[0m)")
					} else {
						printOutTaskResultDetails(5, strings.Replace(fmt.Sprintf("- %s (\033[31mfail\033[0m)", testCase.Result), "exit status ", "exit status: ", 1))

						if verbosity >= 3 {
							printOutTaskResultDetails(4, "")
							if testCase.DebugScript != "" {
								printOutTaskResultDetails(3, "debug_script:")
								printOutTaskResultDetails(5, "script", testCase.DebugScript)
								printOutTaskResultDetails(5, "stdout", testCase.DebugStdout)

								if testCase.DebugResult == nil {
									printOutTaskResultDetails(5, "- exit status: 0")
								} else {
									printOutTaskResultDetails(5, strings.Replace(fmt.Sprintf("- %s", testCase.DebugResult), "exit status ", "exit status: ", 1))
								}
							} else {
								printOutTaskResultDetails(3, "debug_script: \"\" (null)")
							}
						}
					}

					if len(testCase.After) > 0 {
						printOutTaskResultDetails(4, "")
						printOutTaskResultDetails(3, "post-tasks:")
					}

					for _, name := range testCase.After {
						printOutTaskResultDetails(5, "- name: "+name)
						printOutTaskResultDetails(7, "script", c.Cases[c.getIdByName(name)].Script)
						printOutTaskResultDetails(7, "stdout", c.Cases[c.getIdByName(name)].Stdout)

						if c.Cases[c.getIdByName(name)].Result == nil {
							printOutTaskResultDetails(7, "- exit status: 0 (\033[32msuccess\033[0m)")
						} else {
							printOutTaskResultDetails(7, strings.Replace(fmt.Sprintf("- %s (\033[31mfail\033[0m)", c.Cases[c.getIdByName(name)].Result), "exit status ", "exit status: ", 1))
						}
					}

					log.Println()
				}
			}
			return
		}
	}
}

func (c *suitConfig) execTask(item int) {
	testCase := &c.Cases[item]
	if testCase.Script != "" {
		taskStartTime := time.Now()

		for _, name := range testCase.Before {
			c.Cases[c.getIdByName(name)].RunBash()
		}

		testCase.RunBash()

		for _, name := range testCase.After {
			c.Cases[c.getIdByName(name)].RunBash()
		}

		testCase.Duration = duration(taskStartTime, time.Now())
	}
}

func (t *suitConfig) getConf(config string, taskFilter ...string) *suitConfig {
	yamlFile, err := os.ReadFile(config)

	if err != nil {
		log.Fatal(err)
	}

	err = yaml.Unmarshal(yamlFile, t)
	if err != nil {
		log.Fatalf(fmt.Sprintf("Cannot recognize configuration structure in %s file: ", config))
	}

	var envs map[string]string
	wdir, _ := os.Getwd()
	if workdir != "" {
		wdir = workdir
	}

	a := &suitConfig{
		Name:  (*t).Name,
		Cases: []ScenarioItem{},
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

		if len((*t).Cases[i].Loop.Items) > 0 || len((*t).Cases[i].Loop.Command) > 0 {
			Items := []string{}

			if len((*t).Cases[i].Loop.Items) > 0 {
				Items = (*t).Cases[i].Loop.Items
			}

			if len((*t).Cases[i].Loop.Command) > 0 {
				s := (*t).Cases[i]
				s.Script = s.Loop.Command
				stdout, _ := s.RunBash()

				for _, item := range strings.Split(string(stdout), "\n") {
					if item != "" {
						Items = append(Items, item)
					}
				}
			}

			for _, item := range Items {
				last := len(a.Cases)
				(*a).Cases = append(a.Cases, (*t).Cases[i])
				(*a).Cases[last].Case = fmt.Sprintf("%s, item => \"%s\"", (*t).Cases[i].Case, item)
				if (*a).Cases[last].GlobalEnv == nil {
					(*a).Cases[last].GlobalEnv = make(map[string]string)
				}
				(*a).Cases[last].GlobalEnv["item"] = item
			}

		} else {
			(*a).Cases = append(a.Cases, (*t).Cases[i])
		}

	}
	*t = *a
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

func duration(start time.Time, finish time.Time) string {
	return finish.Sub(start).Truncate(time.Millisecond).String()
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
			TotalTime:   c.duration,
			TimeStamp:   time.Now().Format("2006-01-02T15:04:05"),
			Verbosity:   0,
		}

		funcMap := template.FuncMap{
			"Quote": func(m string) string {
				return strconv.Quote(m)
			},
		}

		reportFile, err := os.Create(reportFile)
		defer reportFile.Close()
		if err != nil {
			log.Println(err)
			return
		}

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
					Duration: c.Cases[id].Duration,
				}

				if (verbosity > 1 && c.Cases[id].IsFailed()) || (verbosity > 2) {
					t.Stdout = c.Cases[id].Stdout
				}

				jsonReportData.Tests = append(jsonReportData.Tests, t)
			}
		}
	}

	jsonReportData.Summary = TestsSummary{
		Success:  c.successfull,
		Failed:   c.failed,
		Rating:   c.score,
		Duration: c.duration,
	}

	reportJson, _ := json.MarshalIndent(jsonReportData, "", "  ")
	os.WriteFile(reportFile, reportJson, 0644)
}

var (
	localConfig  = flag.String("c", "", "Local tests case file path (Required unless -C cpecified)")
	remoteConfig = flag.String("C", "", "Remote tests case file url (Required unless -c specified)")
	filter       = flag.String("f", "", "Run tests by name regexp match")
	wdir         = flag.String("w", "", "Set working Dir")
	reportFlag   = flag.String("o", "", "JSON or JUnit report file")
)

var verbosity int = 0

func customUsage() {
	fmt.Fprintf(flag.CommandLine.Output(), "Usage of ./%s:\n", filepath.Base(os.Args[0]))
	flag.PrintDefaults()
	fmt.Println("  -vX")
	fmt.Println("        Verbosity level. Can be:")
	fmt.Println("          -v (-v1, --verbosity=1),")
	fmt.Println("          -vv (-v2, --verbosity=2),")
	fmt.Println("          -vvv (-v3, --verbosity=3)")
	fmt.Println("\nMore details: https://github.com/sbeliakou/check-up/")
}

func listFiles(path string) []string {
	f, _ := os.Stat(path)
	if f.IsDir() {
		path = fmt.Sprintf("%s/*.yml", path)
	}

	files, _ := filepath.Glob(path)
	result := []string{""}

	for _, file := range files {
		f, _ := os.Stat(file)
		if !f.IsDir() {
			if len(file) > 0 {
				result = append(result, file)
			}
		} else {
			result = append(result, listFiles(file)...)
		}
	}
	return result
}

func main() {
	// Modified Args slice
	args := os.Args[:1] // keep the program name
	verbosity = 0
	verbosityRegexp := regexp.MustCompile(`^-v(v+)?=?(\d+)?$|^--verbosity=(\d+)$`)
	for _, arg := range os.Args[1:] {
		matches := verbosityRegexp.FindStringSubmatch(arg)
		if len(matches) > 0 {
			if matches[1] != "" { // Handle -vv, -vvv (matches multiple 'v' after the first)
				verbosity = len(matches[1]) + 1 // +1 because the first 'v' wasn't counted in matches[1]
			} else if matches[2] != "" { // Handle -v=2, -v=3, etc.
				verbosity, _ = strconv.Atoi(matches[2])
			} else if matches[3] != "" { // Handle --verbosity=2, --verbosity=3, etc.
				verbosity, _ = strconv.Atoi(matches[3])
			}
		} else {
			args = append(args, arg) // include other args to be parsed by flag package
		}
	}

	os.Args = args

	flag.Usage = customUsage
	flag.Parse()

	workdir = *wdir

	report.parse(*reportFlag)

	log.SetFlags(0)

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
			log.Println("Using temporary file for configuration:", tmpFile.Name())
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
		log.Fatal("Missing required flags: either -c or -C must be specified.")
	}
}

func handleScenarios(c *suitConfig) {
	c.printHeader()
	if c.getScenarioCount() > 0 {
		max := 30
		for i, id := range c.getScenarioIds() {
			task_title := fmt.Sprintf("   %d/%d  %s", i, c.getScenarioCount(), c.Cases[id].Case)
			if max < len(task_title) {
				max = len(task_title)
			}
		}

		fmt.Println(strings.Repeat("-", max+7))
		for i, id := range c.getScenarioIds() {
			c.execTask(id)
			if c.Cases[id].CanShow() {
				c.printTestStatus(id, i+1)
			}
		}

		fmt.Println(strings.Repeat("-", max+7))
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
