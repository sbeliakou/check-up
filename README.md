# Check-up: Bash-driven Automated Testing System

Check-up is a testing framework for Bash. It provides a simple way to verify that the UNIX programs you write behave as expected.

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

## Running tests

To run your tests, invoke the `checkup` interpreter with a path to a test file.

```
$ checkup -c examples/1_simple_suit.yml 
2021/03/15 14:47:29 -----------------------------------------------------------------------------------
2021/03/15 14:47:29 Running 'Simple Test', 1..3 tests
2021/03/15 14:47:29 -----------------------------------------------------------------------------------
2021/03/15 14:47:29 ✗ [Simple Test] -> Check if '/tmp/dir1' created
2021/03/15 14:47:29 ✓ [Simple Test] => Check if '/opt' created (1), 10ms
2021/03/15 14:47:29 ✗ [Simple Test] -> Check if 'user1' exists
2021/03/15 14:47:29 -----------------------------------------------------------------------------------
2021/03/15 14:47:29 Tests Summary:
2021/03/15 14:47:29   1 (of 3) tests passed, 2 tests failed; rated as 33.33%
2021/03/15 14:47:29 
2021/03/15 14:47:29 Time Spent:  42ms
2021/03/15 14:47:29 -----------------------------------------------------------------------------------
```

```
$ checkup -c examples/2_simple_suit.yml
2021/03/15 15:25:05 -----------------------------------------------------------------------------------
2021/03/15 15:25:05 Running 'Simple Test 2', 1..4 tests
2021/03/15 15:25:05 -----------------------------------------------------------------------------------
2021/03/15 15:25:06 ✗ [Simple Test 2] -> Should finish successfully
2021/03/15 15:25:07 ✗ [Simple Test 2] -> Should create "test-user"
2021/03/15 15:25:07 ✗ [Simple Test 2] -> Should create "unwilling-user"
2021/03/15 15:25:08 ✓ [Simple Test 2] => Shouldn't create "unwilling-user" (1), 352ms
2021/03/15 15:25:08 -----------------------------------------------------------------------------------
2021/03/15 15:25:08 Tests Summary:
2021/03/15 15:25:08   1 (of 4) tests passed, 3 tests failed; rated as 25.00%
2021/03/15 15:25:08 
2021/03/15 15:25:08 Time Spent:  2.846s
2021/03/15 15:25:08 -----------------------------------------------------------------------------------
```

Actually, the script we are testing is located in different directory:
```
checkup -c examples/2_simple_suit.yml -w examples/workspace/
2021/03/15 15:24:40 -----------------------------------------------------------------------------------
2021/03/15 15:24:40 Running 'Simple Test 2', 1..4 tests
2021/03/15 15:24:40 -----------------------------------------------------------------------------------
2021/03/15 15:24:42 ✓ [Simple Test 2] => Should finish successfully (1), 647ms
2021/03/15 15:24:42 ✓ [Simple Test 2] => Should create "test-user" (1), 473ms
2021/03/15 15:24:43 ✗ [Simple Test 2] -> Should create "unwilling-user"
2021/03/15 15:24:43 ✓ [Simple Test 2] => Shouldn't create "unwilling-user" (1), 339ms
2021/03/15 15:24:44 -----------------------------------------------------------------------------------
2021/03/15 15:24:44 Tests Summary:
2021/03/15 15:24:44   3 (of 4) tests passed, 1 tests failed; rated as 75.00%
2021/03/15 15:24:44 
2021/03/15 15:24:44 Time Spent:  3.212s
2021/03/15 15:24:44 -----------------------------------------------------------------------------------
```

### Increasing verbosity

```
$ checkup -c examples/2_simple_suit.yml -v2
2021/03/15 15:26:58 config: examples/2_simple_suit.yml
2021/03/15 15:26:58 verbosity: 2
2021/03/15 15:26:58 -----------------------------------------------------------------------------------
2021/03/15 15:26:58 Running 'Simple Test 2', 1..4 tests
2021/03/15 15:26:58 -----------------------------------------------------------------------------------
2021/03/15 15:27:00 ✗ [Simple Test 2] -> Should finish successfully
2021/03/15 15:27:00   Result: exit status 1
2021/03/15 15:27:00   Output:
bash: ./create_user.sh: No such file or directory
(run, current_dir) =>  docker exec test-server bash ./create_user.sh test-user
rc: 127
output: ''
CMD Failed:
2021/03/15 15:27:00 ✗ [Simple Test 2] -> Should create "test-user"
2021/03/15 15:27:00   Result: exit status 1
2021/03/15 15:27:00   Output:
id: test-user: no such user
(run, current_dir) =>  docker exec test-server id test-user
rc: 1
output: ''
CMD Failed:
2021/03/15 15:27:00 ✗ [Simple Test 2] -> Should create "unwilling-user"
2021/03/15 15:27:00   Result: exit status 1
2021/03/15 15:27:00   Output:
id: unwilling-user: no such user
(run, current_dir) =>  docker exec test-server id unwilling-user
rc: 1
output: ''
CMD Failed:
2021/03/15 15:27:01 ✓ [Simple Test 2] => Shouldn't create "unwilling-user" (1), 343ms
2021/03/15 15:27:01 -----------------------------------------------------------------------------------
2021/03/15 15:27:01 Tests Summary:
2021/03/15 15:27:01   1 (of 4) tests passed, 3 tests failed; rated as 25.00%
2021/03/15 15:27:01 
2021/03/15 15:27:01 Time Spent:  2.683s
2021/03/15 15:27:01 -----------------------------------------------------------------------------------
```