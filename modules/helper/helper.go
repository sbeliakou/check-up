package helper

import "log"

const SampleTestFile = `name: Sample Tests
cases:
- case: /tmp/dir1 exists
  script: |
    [ -d /tmp/dir1 ]

- case: user1 exists
  script: id user1

- case: check something using custom env variables
  script: |
    printenv REGION | grep eu-west-1
  env:
    REGION: eu-west-1

- case: some.service is active
  script: systemctl is-active some.service
  debug_script: |
    journalctl -u some.service

- case: check services from a list
  script: |
    echo $item
    systemctl is-active $item
  debug_script: |
    echo $item
    journalctl -u $item
  loop:
    items:
      - one.service
      - another.service

- case: check services from a command
  script: |
    echo $item
    systemctl is-active $item
  debug_script: |
    echo $item
    journalctl -u $item
  loop:
    command: |
      for i in one.service another.service
      do 
        echo $i; 
      done

- case: restrict script execution time
  script: echo all good
  timeout: 3

- case: no time restriction on script execution
  script: |
    sleep 10
    sleep 20
    exit 1
  timeout: 0 # means the same as unset
`

func CustomUsage() {
	usage := `Usage of ./checkup:
  
    ./checkup -c filename|directory other options
    ./checkup -C url other options

Mandatory Options (One of them):
          
    -C <url>
          Specify the remote test case file URL. Required unless -c is specified.
        
    -c <path>
          Specify the local test case file path. Required unless -C is specified.
        
    -g
          Generate sample test case file.

Other Options:
          
    -f <regexp>
          Run tests matching the specified regular expression for test names.
          
    -o <format=filename>
          Output the test results to a file. Supports JSON or JUnit formats.
          
          Suppoerted formats:
          - json
          - junit
          
    -w <directory>
          Set the working directory for the test execution context.
          
    --version
          Show current version
          
    -v, --verbosity
          Set the verbosity level to control the amount and type of output:
          
          -v=0, --verbosity=0:
              Standard output. Provides essential information without additional details.
              
          -v=1, --verbosity=1:
              Detailed output. Shows comprehensive details about the execution of tasks that fail.
              
          -v=2, --verbosity=2:
              Enhanced Detailed output. Provides comprehensive details on the execution of
              all tasks, whether successful or failed.
              
          -v=3, --verbosity=3:
              Full Detailed output. Shows exhaustive details about the execution of all tasks, 
              including any associated pre-task and post-task activities. Additionally, 
              for failed tasks, it includes detailed debug information.
            
        -v=4, --verbosity=4:
            Debug-Level Detailed output. This level includes all details provided at level 3, 
            plus it displays environment variables specifically for the tasks that fail.

Examples:

  ./checkup -g > tests.yaml
      Generates sample tests file

  ./checkup -c tests.yaml
      Runs all tasks from the file, shows minimal details, just statuses of tasks execution and summary report
      
  ./checkup -c tests.yaml -v3
      Runs all tasks from the file, shows detailed output of all bash tasks execution: stdout, responce code.
      If 'debug_script' is set, it will also runt for early troubleshooting
      
  ./checkup -c tests.yaml -f user 
      Runs only those tasks which "case:" field contains word "user"

Additional Information:
  Complete documentation and more details are available at:
  https://github.com/sbeliakou/check-up/
`

	log.Println(usage)
}
