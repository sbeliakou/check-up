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
$ [ Simple Test ], 1..3 tests
-----------------------------------------------------------------------------------
✗  1  Check if '/tmp/dir1' created, 6ms
✓  2  Check if '/opt' created, 7ms
✗  3  Check if 'user1' exists, 130ms
-----------------------------------------------------------------------------------
1 (of 3) tests passed, 2 tests failed, rated as 33.33%, spent 144ms
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

### Increasing verbosity

```
$ checkup -c examples/2_simple_suit.yml -v2
[ Simple Test 2 ], 1..4 tests
-----------------------------------------------------------------------------------
✗  1  Should finish successfully, 326ms
~~~~~
>> stdout:
bash: ./create_user.sh: No such file or directory
(run, /Users/sbeliakou/ws/github/playpit-labs/check-up) =>  docker exec test-server bash ./create_user.sh test-user
rc: 127
output: ''
CMD Failed:
>> exit status 1 (failure)
~~~~~
✗  2  Should create "test-user", 383ms
~~~~~
>> stdout:
id: test-user: no such user
(run, /Users/sbeliakou/ws/github/playpit-labs/check-up) =>  docker exec test-server id test-user
rc: 1
output: ''
CMD Failed:
>> exit status 1 (failure)
~~~~~
✗  3  Should create "unwilling-user", 324ms
~~~~~
>> stdout:
id: unwilling-user: no such user
(run, /Users/sbeliakou/ws/github/playpit-labs/check-up) =>  docker exec test-server id unwilling-user
rc: 1
output: ''
CMD Failed:
>> exit status 1 (failure)
~~~~~
✓  4  Shouldn't create "unwilling-user", 262ms
-----------------------------------------------------------------------------------
1 (of 4) tests passed, 3 tests failed, rated as 25.00%, spent 2.208s
```