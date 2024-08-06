# Check-up: A Lightweight, Bash-friendly Powerful Automated Testing Tool for UNIX/Linux Systems

Check-up leverages a powerful testing framework specifically designed for UNIX/Linux systems, facilitating seamless validation of system configurations and functionalities across diverse environments. Utilizing a YAML-based scripting format, the tool integrates bash-friendly syntax within test case definitions, enabling straightforward and intuitive scripting that aligns closely with typical UNIX/Linux administration tasks.

### Key Features:

- **Automated Test Execution:** Executes complex test suites smoothly using predefined YAML scripts.
- **Flexible Environment Management:** Utilizes pre-test and post-test scripts to set up and clean up testing environments, ensuring effective isolation and minimized side effects.
- **Execution Time Controls:** Implements timeouts for individual tests to prevent extended run times and supports global time restrictions to maintain overall test efficiency.
- **Loop Iterations:** Enables repetitive testing over predefined or dynamically generated data sets, reducing code redundancy and enhancing test coverage.
- **Selective Task Skipping:** Provides functionality to skip tests based on specified conditions like empty scripts or explicit flags, optimizing test execution.
- **Enhanced Debugging Tools:** Includes debug scripts that run automatically upon test failures, offering real-time insights for faster troubleshooting.
- **Customizable Outputs:** Allows users to modify test case titles in output reports and adjust verbosity for detailed results.
- **Report Generation in Multiple Formats:** Besides standard and detailed console outputs, Check-up supports exporting test results in both JSON and JUnit XML formats, facilitating integration with continuous integration (CI) tools and extending its utility in automated workflows.

### Lightweight Design and Easy Deployment:

- **Go-Based Architecture:** Check-up is built in Go, making it lightweight and robust, without the need for installing multiple large frameworks. Its compact and standalone nature ensures quick setup and low overhead, further enhancing the user experience and performance.
- **Container-Friendly:** Ideal for containerized environments, Check-up can be easily included in Docker containers or used in CI/CD pipelines, providing flexible and scalable testing solutions.

### Integration with DevOps Practices

- Designed to fit naturally within CI/CD pipelines, Check-up is ideal for automated deployment environments, providing rigorous configuration validation and operational assurance without disrupting existing processes.
By combining a bash-friendly approach with sophisticated testing functionalities, Check-up serves as an indispensable tool for system administrators, DevOps engineers, and developers. Its ability to integrate closely with UNIX/Linux system management practices while remaining lightweight and easy to deploy makes it a top choice for ensuring system reliability and performance in complex IT environments.

## Installation

Install `checkup` by downloading the release binary and setting executable permissions:

```bash
# Replace $RELEASE_ID with the desired release ID from GitHub Releases page
$ wget https://github.com/sbeliakou/check-up/releases/download/$RELEASE_ID/checkup-linux -O /usr/bin/checkup
$ chmod +x /usr/bin/checkup
```

For more information on releases, as well as version compatibility, visit: [Check-up Releases](https://github.com/sbeliakou/check-up/releases)

## Writing Tests

Test files are YAML-format containing a list of bash script cases:

```yaml
name: Simple Test Suite
cases:
- case: "Verify existence of '/tmp/dir1'"
  script: |
    test -d /tmp/dir1

- case: "Validate presence of user 'user1'"
  script: |
    id user1
```

Test files are structured to execute scripts during the test case evaluations or as pre and post-operations.

```yaml
name: Simple Test Suite
cases:

- script: >
    docker run -d 
      --name test-server 
      --volume $(pwd):$(pwd) 
      --workdir $(pwd) 
      --privileged quay.io/sbeliakou/ansible-training:centos

- case: "Check successful execution within container"
  script: |
    docker exec test-server bash ./create_user.sh test-user

- case: "Verify 'test-user' creation inside container"
  script: |
    docker exec test-server id test-user

- case: "Attempt to validate unintended user creation"
  script: |
    docker exec test-server id unwilling-user
    [ $? -ne 0 ] && exit 0 || exit 1

# Execute after all test cases
- script: docker rm -f test-server
```

## Framework Syntax Features

### 1. Dependant Tasks Execution

Before and after task execution scripts help set the necessary environment for your tests to run in isolation and clean up when the tests finish.

Example - Ensure dependent containers are set up and cleaned up:

```yaml
# silent task, just for doing some usefull stuff
- name: provision contianer 1
  script: docker run --name test1 -d centos:7 sleep infinity

# silent task, just for doing some usefull stuff
- name: provision contianer 2
  script: docker run --name test2 -d centos:7 sleep infinity

# silent task, just for doing some usefull stuff
- name: clean up containers
  script: |
    docker stop test1 && docker rm test1
    docker stop test2 && docker rm test2

- case: docker run
  script: |
    docker ps
    docker run --name test -d centos:7 sleep infinity
  before:
    - clean up containers
    - provision contianer 1
    - provision contianer 2
  after:
    - clean up containers

- name: create '/tmp/new_folder' folder
  script: mkdir -p /tmp/new_folder

- name: delete '/tmp/new_folder' folder
  script: rm -rf /tmp/new_folder

# before running this task checkup runs "before" tasks
# and after it finishes, it executes "after" tasks
- case: Validate that '/tmp/new_folder' dir exists
  script: test -d /tmp/new_folder
  before:
    - create '/tmp/new_folder' folder
    - create file in '/tmp/new_folder' folder
  after:
    - delete '/tmp/new_folder' folder
```

### 2. Execution Time Restrictions
Define timeouts for each test case, limiting how long it can run to prevent stalling.

Example - Set execution timeout:

```yaml
- case: "Long running task gets terminated"
  script: sleep 20
  timeout: 5
```

Also, it's possible to restrict every task execution time by setting `-t=<seconds>` option when running `checkup`:

```bash
./checkup -c tests.yaml -t=5
```

### 3. Iterating Over a Loop
Allows iterating tests over dynamic or predefined data sets.

Example - Verify multiple services:

```yaml
- case: "Check if service '$item' is active"
  script: systemctl is-active $item
  loop:
    items:
      - httpd.service
      - sshd.service

- case: "Check if service '$item' is active"
  script: systemctl is-active $item
  loop:
    command: |
      cd /etc/systemd/system/
      ls *.service
```

Here, the `$item` acts as a variable placeholder for each loop iteration.


### 4. Skipping Tasks
Selective task execution based on script content or explicit flags.

Example - Conditions for skipping tasks:
```yaml
- case: "This task will be skipped since the script is empty"
  script: 

- case: "Intentionally skipped task"
  script: echo "This script won’t be executed."
  skip: true
```

### 5. Filtering tasks
To run only necessary tasks from the suit `-f=<pattern>` option should be provided:

Example - A few tasks:
```yaml
...

- case: nginx service is active
  script: systemctl is-active nginx

- case: uwsgi service is active
  script: systemctl is-active uwsgi

...
```

```bash
# run those tasks which has "service" in the name
./checkup -c tests.yaml -f=service
```

### 5. Setting custom Environment variables for specific tasks or globally for the tasks suit
Sometimes it's essential to run tasks with some additional Environmental variables

Example - Setting Enviroment Variables:
```yaml
- case: check something using custom env variables
  script: |
    printenv REGION | grep $REGION
  env:
    REGION: eu-west-1
    ZONE: eu-west-1a
```

### 6. Adding debug script to easily troubleshoot why the task is failing
Debug commands can be easily added to the test suite. The script will be run only when the main task fails.

To see the result of the execution of `debug` script, the verbosity level should be `3` or higher

Example - Running additional commands when the main task fails:

```yaml
- case: check services from a command
  script: |
    systemctl is-active $item
  debug:
    script: |
      journalctl -u $item
    timeout: 10
```

Running the checkup tool:

```bash
./checkup -c tests.yaml -v=3
./checkup -c tests.yaml -v=4
```

### 7. Customizing task titles in reports

By default, the report looks as the following:

```
[ 1. Initial Setup. Catalog: Filesystem ], 1..10 tests, file: cis-benchmark/rhel/1/1.1.yaml
------------------------------------------------------------------------------------
✗  1/10  Ensure mounting of "cramfs" is disabled, 78ms
✗  2/10  Ensure mounting of "squashfs" is disabled, 32ms
✗  3/10  Ensure mounting of "udf" is disabled, 44ms
✗  4/10  Ensure /tmp is configured, 68ms
✓  5/10  Ensure nodev, nosuid, noexec option set on "/tmp" partition, 81ms
✓  6/10  Ensure nodev, nosuid, noexec option set on "/var/tmp" partition, 82ms
-  7/10  Ensure nodev, nosuid, noexec option set on removable media partition, skipping reason: empty 'script' setting 
✓  8/10  Ensure sticky bit is set on all world-writable directories, 144ms
✗  9/10  Disable Automounting, 32ms
✗ 10/10  Ensure mounting of usb-storage is disabled, 33ms
------------------------------------------------------------------------------------
3 (of 10) tests passed, 6 tests failed, 1 tests skipped, rated as 33.33%, spent 598ms
```

This is the way how to update their indexes:
```
name: "1. Initial Setup. Catalog: Filesystem"
custom_index: "1.1.{{ .TaskId }}"
cases:
...
```

The following variables are currently supported:

- `.TaskId` - current task Id, starts from 0
- `.TaskCount` - total amount of tasks


And the output changes accordingly:
```
[ 1. Initial Setup. Catalog: Filesystem ], 1..10 tests, file: cis-benchmark/rhel/1/1.1.yaml
------------------------------------------------------------------------------------
✗ 1.1.0 Ensure mounting of "cramfs" is disabled, 43ms
✗ 1.1.1 Ensure mounting of "squashfs" is disabled, 38ms
✗ 1.1.2 Ensure mounting of "udf" is disabled, 37ms
✗ 1.1.3 Ensure /tmp is configured, 51ms
✓ 1.1.4 Ensure nodev, nosuid, noexec option set on "/tmp" partition, 95ms
✓ 1.1.5 Ensure nodev, nosuid, noexec option set on "/var/tmp" partition, 86ms
- 1.1.6 Ensure nodev, nosuid, noexec option set on removable media partition, skipping reason: empty 'script' setting 
✓ 1.1.7 Ensure sticky bit is set on all world-writable directories, 138ms
✗ 1.1.8 Disable Automounting, 31ms
✗ 1.1.9 Ensure mounting of usb-storage is disabled, 33ms
------------------------------------------------------------------------------------
3 (of 10) tests passed, 6 tests failed, 1 tests skipped, rated as 33.33%, spent 557ms
```

## Checkupt Command-line Options:

### Mandatory Options (One of them):

- `-C <url>` - Specify the remote test case file URL. Required unless -c is specified.
- `-c <path>` - Specify the local test case file path. Required unless -C is specified.
- `-g` - Generate sample test case file.

### Other Options:

- `-w` - Sets default working dir for the tasks
- `-f <regexp>` - Run tests matching the specified regular expression for test names.
- `-o <format=filename>` - Output the test results to a file. Supports JSON or JUnit formats.
    - `-o json=filename`: Saves the report in JSON format
    - `-o junit=filename`: Saves the report in JUnit format
- `-w <directory>` - Set the working directory for the test execution context.
- `--version` - Show current version
- `-v`, `--verbosity` - Set the verbosity level to control the amount and type of output:  
    - `-v=0`, `--verbosity=0`: Standard output. Provides essential information without additional details.
    - `-v=1`, `--verbosity=1`: Detailed output. Shows comprehensive details about the execution of tasks that fail.
            
    - `-v=2`, `--verbosity=2`: Enhanced Detailed output. Provides comprehensive details on the execution of all tasks, whether successful or failed.
    - `-v=3`, `--verbosity=3`: Full Detailed output. Shows exhaustive details about the execution of all tasks, including any associated pre-task and post-task activities. Additionally, for failed tasks, it includes detailed debug information.
    - `-v=4`, `--verbosity=4`: Debug-Level Detailed output. This level includes all details provided at level 3, plus it displays environment variables specifically for the tasks that fail.


## License

This project is licensed under the MIT License - see the [LICENSE](#mit-license) file for details.

### Included MIT License

#### Grant of License

This License Agreement permits users to use the Check-up Automated Testing Framework on any compatible systems, provided they adhere to the following terms outlined in the MIT License.

#### Scope of Use

You are granted the right to use, modify, and distribute the software freely, subject to the conditions listed below under the MIT License.

#### MIT License

```markdown
MIT License

Copyright (c) 2024 Siarhei Beliakou

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

By using the Check-up Automated Testing Framework, you acknowledge that you have read, understood, and agreed to be bound by the terms of this MIT License.
