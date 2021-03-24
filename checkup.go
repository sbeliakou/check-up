package main

import (
	"encoding/json"
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

	"./templates/bash"
	"./templates/jUnit"
)

var verbosity int = 0
var workdir string = ""

func print(msg string) {
	if os.Getenv("TERM") == "" {
		mod := regexp.MustCompile(`\033[^m]*m`).ReplaceAllString(msg, "")
		mod = regexp.MustCompile(`✓`).ReplaceAllString(mod, "ok")
		mod = regexp.MustCompile(`✗`).ReplaceAllString(mod, "NO")
		log.Println(mod)
	} else {
		log.Println(msg)
	}

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
	Debug       string            `yaml:"debug"`
	Before      []string          `yaml:"before"`
	After       []string          `yaml:"after"`

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
	var stdout []byte = []byte("")
	var err error = nil

	if s.Script != "" {
		tmpDir, _ := ioutil.TempDir("/var/tmp", "._")
		defer os.RemoveAll(tmpDir)

		tmpFile, _ := ioutil.TempFile(tmpDir, "tmp.*")

		T := struct {
			Script string
		}{
			Script: s.Script,
		}

		tmpl, _ := template.New("bash-script").Parse(string(bash.BashScript))
		tmpl.Execute(tmpFile, T)

		script := exec.Command("/bin/bash", tmpFile.Name())
		script.Dir = workdir
		script.Env = os.Environ()
		for key, value := range s.GlobalEnv {
			script.Env = append(script.Env,
				fmt.Sprintf("%s=%s", key, value),
			)
		}

		stdout, err = script.CombinedOutput()
		s.Stdout = strings.TrimSpace(string(stdout))
		s.Result = err

		if err == nil {
			s.Status = "success"
		} else {
			s.Status = "failed"
		}

		return stdout, err
	}
	return []byte(""), nil
}

type suitConfig struct {
	Name  string         `yaml:"name"`
	Cases []ScenarioItem `yaml:"cases"`

	filter string

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

	c.startTime = time.Now()

	if scenariosCount > 1 {
		log.Printf("[ %s ], 1..%d tests\n", c.Name, scenariosCount)
		return
	}

	if scenariosCount == 1 {
		log.Printf("[ %s ], 1 test\n", c.Name)
		return
	}

	if scenariosCount == 0 {
		log.Printf("[ %s ], no tests to run\n", c.Name)
		return
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
					print(fmt.Sprintf("\033[32m✓ %2d  %s, %s\033[0m", i, testCase.Case, testCase.Duration))
				} else {
					print(fmt.Sprintf("\033[31m✗ %2d  %s, %s\033[0m", i, testCase.Case, testCase.Duration))
				}

				if (verbosity > 1 && testCase.IsFailed()) || (verbosity > 2) {
					for _, name := range testCase.Before {
						log.Printf("(run: %s)\n", name)
						log.Printf(">> script:\n%s\n", strings.TrimSpace(c.Cases[c.getIdByName(name)].Script))
						log.Printf(">> stdout:\n%s\n", c.Cases[c.getIdByName(name)].Stdout)
						if c.Cases[c.getIdByName(name)].Result == nil {
							log.Printf(">> exit status 0 (successfull)")
						} else {
							log.Printf(">> %s (failure)", c.Cases[c.getIdByName(name)].Result)
						}
						log.Printf("---")
					}

					log.Printf("~~~~~")
					log.Printf(">> stdout:\n%s", strings.TrimSpace(testCase.Stdout))
					if testCase.Result == nil {
						log.Printf(">> exit status 0 (successfull)")
					} else {
						log.Printf(">> %s (failure)", testCase.Result)
					}
					log.Printf("~~~~~")

					for _, name := range testCase.After {
						log.Printf("(run: %s)\n", name)
						log.Printf(">> script:\n%s\n", strings.TrimSpace(c.Cases[c.getIdByName(name)].Script))
						log.Printf(">> stdout:\n%s\n", c.Cases[c.getIdByName(name)].Stdout)
						if c.Cases[c.getIdByName(name)].Result == nil {
							log.Printf(">> exit status 0 (successfull)")
						} else {
							log.Printf(">> %s (failure)", c.Cases[c.getIdByName(name)].Result)
						}
						log.Printf("---")
					}
				}
			}
			return
		}
	}
}

func (c *suitConfig) exec(item int) {
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

		// if (*t).Cases[i].Log != "" {
		// 	logging := (*t).Cases[i].Log
		// 	if logging == "True" || logging == "true" || logging == "Yes" || logging == "yes" {
		// 		(*t).Cases[i].Log = "true"
		// 	}
		// }

		if (*t).Cases[i].CanShow() {
			if (*t).Cases[i].Weight == 0 {
				(*t).Cases[i].Weight = 1
			}
		}
	}
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
	return
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
	ioutil.WriteFile(reportFile, reportJson, 0644)
}

func main() {
	localConfig := flag.String("c", "", "Local config path")
	remoteConfig := flag.String("C", "", "Remote config url")

	filter := flag.String("f", "", "Run tests by regexp match")
	wdir := flag.String("w", "", "Set working Dir")

	// -o junit=...
	// -o json=...
	reportFlag := flag.String("o", "", "JSON or JUnit report file")

	// -v1 show description if it's set
	// -v2 show failed outputs
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

	report.parse(*reportFlag)

	// Set Log Level
	// https://golang.org/pkg/log/#example_Logger
	log.SetFlags(0)

	verbosity = func() int {
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

	if *localConfig == "" && *remoteConfig == "" {
		log.Fatal("Please specify -c or -C")
	}

	if len(regexp.MustCompile("^http(s)?:").FindStringSubmatch(*localConfig)) > 0 {
		load(tmpFile, *localConfig)
		localConfig = &tmpFileName
		log.Println("tmpFile:", tmpFile.Name())
	}

	var c suitConfig
	c.getConf(*localConfig, *filter)

	c.printHeader()
	if c.getScenarioCount() > 0 {
		print("-----------------------------------------------------------------------------------")
		i := 1
		for _, id := range c.getScenarioIds() {
			c.exec(id)
			if c.Cases[id].CanShow() {
				c.printTestStatus(id, i)
				i++
			}
		}
		print("-----------------------------------------------------------------------------------")
	}
	c.signOff()
	c.printSummary()

	switch report.format {
	case "junit":
		jUnitReportSave(report.fileName, c)
	case "json":
		jsonReportSave(report.fileName, c)
	}
}
