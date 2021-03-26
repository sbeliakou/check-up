# Check-up: Bash-driven Automated Testing System

Check-up is a testing framework for Bash. It provides a simple way to verify that the UNIX programs you write behave as expected.

## Installation

You can find all releases here: https://github.com/sbeliakou/check-up/releases

```
$ wget https://github.com/sbeliakou/check-up/releases/download/${TAG_ID}/checkup-linux -o /usr/bin/checkup
$ chmod a+x /usr/bin/checkup
```

## Writing tests

A test file is a list of bash-scripts in a form of YAML structure:

```yaml
name: Simple Test 1
cases:
- case: "Check if '/tmp/dir1' created"
  script: |
    [ -d /tmp/dir1 ]

- case: "Check if 'user1' exists"
  script: id user1
```

In some cases you'd like to run scripts before executing tests cases. It's useful if you need to install some software/tools or for example run docker container.

```yaml
name: Simple Test 2
cases:

- script: >
    docker run -d 
    --name test-server 
    --volume $(pwd):$(pwd) 
    --workdir $(pwd) 
    --privileged 
    quay.io/sbeliakou/ansible-training:centos

- case: Should finish successfully
  script: |
    run docker exec test-server bash ./create_user.sh test-user
    assert_success

- case: Should create "test-user"
  script: |
    run docker exec test-server id test-user
    assert_success

# Obviously, it shouldn't, but let's check
- case: Should create "unwilling-user"
  script: |
    run docker exec test-server id unwilling-user
    assert_success

- case: Shouldn't create "unwilling-user"
  script: |
    run docker exec test-server id unwilling-user
    assert_failure

- script: docker rm -f test-server
```

### Test-Case Samples

Expecting command finishes successfully
```yaml
- case: expecting command finishes successfully (bash way)
  script: |
    command args

- case: expecting command finishes successfully (the same as above, bats way)
  script: |
    run command args
    assert_success
```

Expecting the command fails
```yaml
- case: expecting command fails (bats way)
  script: |
    run command args
    assert_failure
```

Expecting the command prints out the exact value
```yaml
- case: expecting command prints the exact value (bash way)
  script: |
    echo "exact value" | grep -w "exact value"

- case: expecting command prints the exact value (the same as above, bats way)
  script: |
    run echo "exact value"
    assert_output "exact value"
```

Expecting the command prints out some message
```yaml
- case: expecting command prints some message (bash way)
  script: |
    echo "the message can be like this one: 'hello world!'" | grep "hello world"

- case: expecting command prints some message (the same as above, bats way)
  script: |
    run echo "the message can be like this one: 'hello world!'"
    assert_output --partial "hello world!"
```
## Running tests

To run your tests, invoke the `checkup` interpreter with a path to a test file.

```
$ checkup -c examples/1_simple_suit.yml 
[ Simple Test ], 1..3 tests
-----------------------------------------------------------------------------------
✗  1  Check if '/tmp/dir1' created, 30ms
✓  2  Check if '/opt' created, 4ms
✗  3  Check if 'user1' exists, 115ms
-----------------------------------------------------------------------------------
1 (of 3) tests passed, 2 tests failed, rated as 33.33%, spent 151ms
```

```
$ checkup -c examples/2_simple_suit.yml
[ Simple Test 2 ], 1..4 tests
-----------------------------------------------------------------------------------
✗  1  Should finish successfully, 308ms
✗  2  Should create "test-user", 329ms
✗  3  Should create "unwilling-user", 291ms
✓  4  Shouldn't create "unwilling-user", 241ms
-----------------------------------------------------------------------------------
1 (of 4) tests passed, 3 tests failed, rated as 25.00%, spent 2.172s
```

Actually, the script we are testing is located in different directory:
```
$ checkup -c examples/2_simple_suit.yml -w examples/workspace/
[ Simple Test 2 ], 1..4 tests
-----------------------------------------------------------------------------------
✓  1  Should finish successfully, 525ms
✓  2  Should create "test-user", 375ms
✗  3  Should create "unwilling-user", 350ms
✓  4  Shouldn't create "unwilling-user", 310ms
-----------------------------------------------------------------------------------
3 (of 4) tests passed, 1 tests failed, rated as 75.00%, spent 2.849s
```

## Types of case items

There're 3 types of items which can be run:

- "background" items/scripts - will run in order of they appear in "cases" list bt they don't impact on final result and won't show in the report. It's useful for operational tasks - to perform something before and after test case. They don't have "case" or "name" tag
- "reference" tasks - will run only on demand, won't impact on result score. They must have "name" tag
- "test case" - actual test case which are shown in tthe report. They must have "case" tag

```yaml
- name: reference task 1
  script: |
    run command 1

- name: reference task 2
  script: |
    run command 2

- script: |
    run operational stuff

- case: test case 1
  script: |
    testting something

- script: |
    run another operational stuff

- case: test case 2
  before:
    - reference task 1
  script: |
    testting something
  after:
    - reference task 2
```

## Options

### Providing Test Case Scenarios and Resources:
- "-c" provide test case file on local filesystem
- "-C" provide test case file on remote by URL
- "-w" set working directory with source files for testing cases

### Verbosity 
- "-v1"/"-v2"/"-v3" verbosity level, shows what happens during test case execution

### Reporting output

- "-o type=file" to save the result to the file of one of types: `json` or `junit`