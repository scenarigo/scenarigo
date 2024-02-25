<a href="https://github.com/zoncoen/scenarigo">
  <p align="center">
    <img alt="Scenarigo" src="https://user-images.githubusercontent.com/2238852/205980597-37eaaf03-fd35-4a04-93c4-884c95f48df3.png" width="485px">
  </p>
</a>

A scenario-based API testing tool for HTTP/gRPC server.

[![godoc](https://godoc.org/github.com/zoncoen/scenarigo?status.svg)](https://pkg.go.dev/github.com/zoncoen/scenarigo)
![test](https://github.com/zoncoen/scenarigo/workflows/test/badge.svg?branch=main)
[![codecov](https://codecov.io/gh/zoncoen/scenarigo/branch/main/graph/badge.svg)](https://codecov.io/gh/zoncoen/scenarigo)
[![go report](https://goreportcard.com/badge/zoncoen/scenarigo)](https://goreportcard.com/report/github.com/zoncoen/scenarigo)
[![codebeat](https://codebeat.co/badges/93ee2453-1a25-4db6-b98e-c430c994b4b8)](https://codebeat.co/projects/github-com-zoncoen-scenarigo-main)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

## Overview

Scenarigo is a scenario-based API testing tool for HTTP/gRPC server.
It is written in Go and provides [a plugin feature](#plugin) that enables you to extend by writing Go code.
You can write test scenarios as YAML files and executes them.

```yaml github.yaml
title: get scenarigo repository
vars:
  user: zoncoen
  repo: scenarigo
steps:
- title: get repository
  protocol: http
  request:
    method: GET
    url: 'https://api.github.com/repos/{{vars.user}}/{{vars.repo}}'
  expect:
    code: OK
    body:
      id: '{{int($) > 0}}'
      name: '{{vars.repo}}'
```

## Installation

### go install command (recommend)

```shell
$ go install github.com/zoncoen/scenarigo/cmd/scenarigo@v0.17.0
```

### from release page

Go to the [releases page](https://github.com/zoncoen/scenarigo/releases) and download the zip file. Unpack the zip file, and put the binary to a directory in your `$PATH`.

You can download the latest command into the `./scenarigo` directory with the following one-liner code. Place the binary `./scenarigo/scenarigo` into your `$PATH`.

```shell
$ version=$(curl -s https://api.github.com/repos/zoncoen/scenarigo/releases/latest | jq -r '.tag_name') && \
    go_version='go1.21.6' && \
    curl -sLJ https://github.com/zoncoen/scenarigo/releases/download/${version}/scenarigo_${version}_${go_version}_$(uname)_$(uname -m).tar.gz -o scenarigo.tar.gz && \
    mkdir ./scenarigo && tar -zxvf ./scenarigo.tar.gz -C ./scenarigo && rm scenarigo.tar.gz
```

**Notes**: If you use the plugin mechanism, the `scenarigo` command and plugins must be built using the same version of Go.

### Setup

You can generate a configuration file `scenarigo.yaml` via the following command.

```shell
$ scenarigo config init
```

```yaml scenarigo.yaml
schemaVersion: config/v1

# global variables
vars:
  endpoint: http://api.example.com

scenarios: [] # Specify test scenario files and directories.

pluginDirectory: ./gen    # Specify the root directory of plugins.
plugins:                  # Specify configurations to build plugins.
  plugin.so:              # Map keys specify plugin output file path from the root directory of plugins.
    src: ./path/to/plugin # Specify the source file, directory, or "go gettable" module path of the plugin.

output:
  verbose: false # Enable verbose output.
  colored: false # Enable colored output with ANSI color escape codes. It is enabled by default but disabled when a NO_COLOR environment variable is set (regardless of its value).
  summary: false # Enable summary output.
  report:
    json:
      filename: ./report.json # Specify a filename for test report output in JSON.
    junit:
      filename: ./junit.xml   # Specify a filename for test report output in JUnit XML format.
```

## Usage

`scenarigo run` executes test scenarios based on the configuration file.

```yaml scenarigo.yaml
schemaVersion: config/v1

scenarios:
- github.yaml
```

```yaml github.yaml
title: get scenarigo repository
steps:
- title: GET https://api.github.com/repos/zoncoen/scenarigo
  vars:
    user: zoncoen
    repo: scenarigo
  protocol: http
  request:
    method: GET
    url: "https://api.github.com/repos/{{vars.user}}/{{vars.repo}}"
  expect:
    code: OK
    body:
      name: "{{vars.repo}}"
```

```shell
$ scenarigo run
ok      github.yaml     0.068s
```

You can see all commands and options by `scenarigo help`.

```
scenarigo is a scenario-based API testing tool.

Usage:
  scenarigo [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  config      manage the scenarigo configuration file
  dump        dump test scenario files
  help        Help about any command
  list        list the test scenario files
  plugin      provide operations for plugins
  run         run test scenarios
  version     print scenarigo version

Flags:
  -c, --config string   specify configuration file path (read configuration from stdin if specified "-")
  -h, --help            help for scenarigo
      --root string     specify root directory (default value is the directory of configuration file)

Use "scenarigo [command] --help" for more information about a command.
```

## How to write test scenarios

You can write test scenarios easily in YAML.

### Send HTTP requests

A test scenario consists of some steps. A step represents an API request. The scenario steps will be run from top to bottom sequentially.
This simple example has a step that sends a `GET` request to `http://example.com/message`.

```yaml
title: check /message
steps:
- title: GET /message
  protocol: http
  request:
    method: GET
    url: http://example.com/message
```

To send a query parameter, add it directly to the URL or use the `query` field.

```yaml
title: check /message
steps:
- title: GET /message
  protocol: http
  request:
    method: GET
    url: http://example.com/message
    query:
      id: 1
```

You can use other methods to send data to your APIs.

```yaml
title: check /message
steps:
- title: POST /message
  protocol: http
  request:
    method: POST
    url: http://example.com/message
    body:
      message: hello
```

By default, Scenarigo will send body data as JSON. If you want to use other formats, set the `Content-Type` header.

```yaml
title: check /message
steps:
- title: POST /message
  protocol: http
  request:
    method: POST
    url: http://example.com/message
    header:
      Content-Type: application/x-www-form-urlencoded
    body:
      message: hello
```

Available `Content-Type` header to encode request body is the following.

- `application/json` (default)
- `text/plain`
- `application/x-www-form-urlencoded`

### Check HTTP responses

You can test your APIs by checking responses. If the result differs expected values, Scenarigo aborts the execution of the test scenario and notify the error.

```yaml
title: check /message
steps:
- title: GET /message
  protocol: http
  request:
    method: GET
    url: http://example.com/message
    query:
      id: 1
  expect:
    code: OK
    header:
      Content-Type: application/json; charset=utf-8
    body:
      id: 1
      message: hello
```

Scenarigo allows [template string](#template-string) as expected values.
Besides, you can write assertions by conditional expressions with the actual value `$`.

```yaml
title: check /message
steps:
- title: GET /message
  protocol: http
  request:
    method: GET
    url: http://example.com/message
    query:
      id: 1
  expect:
    code: OK
    header:
      Content-Type: application/json; charset=utf-8
    body:
      id: {{int($) > 0}}
      message: '{{"hello" + " world"}}'
```

### Variables

The `vars` field defines variables that can be referred by [template string](#template-string) like `'{{vars.id}}'`.

```yaml
title: get message 1
vars:
  id: 1
steps:
- title: GET /messages
  protocol: http
  request:
    method: GET
    url: 'http://example.com/messages/{{vars.id}}'
```

You can define `step` scope variables that can't be accessed from other steps.

```yaml
title: get message 1
steps:
- title: GET /messages
  vars:
    id: 1
  protocol: http
  request:
    method: GET
    url: 'http://example.com/messages/{{vars.id}}'
```

If you want to pass the response data to the subsequent steps, use the `bind` field.

```yaml
title: re-post message 1
vars:
  id: 1
steps:
- title: GET /messages
  protocol: http
  request:
    method: GET
    url: 'http://example.com/messages/{{vars.id}}'
  bind:
    vars:
      msg: '{{response.body.text}}'
- title: POST /messages
  protocol: http
  request:
    method: POST
    url: http://example.com/messages
    header:
      Content-Type: application/json
    body:
      text: '{{vars.msg}}'
  expect:
    code: OK
    body:
      id: '{{assert.notZero}}'
      text: '{{request.body.text}}'
```

### Timeout/Retry

You can set timeout and retry policy for each step.
Duration strings are parsed by [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration).
Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".

```yaml
steps:
- protocol: http
  request:
    method: GET
    url: http://example.com
  expect:
    code: OK
  timeout: 30s           # default values is 0, 0 means no timeout
  retry:                 # default policy is never retry
    constant:
      interval: 5s       # default value is 1s
      maxRetries: 1      # default value is 5, 0 means forever
      maxElapsedTime: 1m # default value is 0, 0 means forever
```

Scenarigo also provides the retry feature with an exponential backoff algorithm.

```yaml
steps:
- protocol: http
  request:
    method: GET
    url: http://example.com
  expect:
    code: OK
  timeout: 30s             # default values is 0, 0 means no timeout
  retry:                   # default policy is never retry
    exponential:
      initialInterval: 1s  # default value is 500ms
      factor: 2            # default value is 1.5
      jitterFactor: 0.5    # default value is 0.5
      maxInterval: 180s    # default value is 60s
      maxRetries: 10       # default value is 5, 0 means forever
      maxElapsedTime: 10m  # default value is 0, 0 means forever
```

The actual interval is calculated using the following formula.

```
initialInterval * factor ^ (retry count - 1) * (random value in range [1 - jitterFactor, 1 + jitterFactor])
```

For example, the retry intervals will be like the following table with the above retry policy.

Note: `maxInterval` caps the retry interval, not the randomized interval.

|Retry #|Retry interval|Randomized interval range|
|---|---|---|
|1|1s|[0.5s, 1.5s]|
|2|2s|[1s, 3s]|
|3|4s|[2s, 6s]|
|4|8s|[4s, 12s]|
|5|16s|[8s, 24s]|
|6|32s|[16s, 48s]|
|7|64s|[32s, 96s]|
|8|128s|[64s, 192s]|
|9|180s|[90s, 270s]|
|10|180s|[90s, 270s]|

### Using conditions to control step execution

You can use `if` field to prevent a step from execution unless a condition is met. The template expression must return a boolean value. For example, you can access the results of other steps like `{{steps.step_id.result}}`. There are three result kinds of steps: `passed`, `failed`, and `skipped`.

Scenarigo doesn't execute subsequent steps if a step fails in default. If you want to continue running the test scenario even if a step fails, set true to the `continueOnError` field.

For example, the second step will be executed in the following test scenario when the first step fails only.

```yaml
schemaVersion: scenario/v1
title: create item if not found
vars:
  itemName: foo
  itemPrice: 100
steps:
- id: find # need to set id to access the result of this step
  title: find by name
  continueOnError: true # the errors of this step don't fail the test scenario
  protocol: http
  request:
    method: GET
    url: 'http://example.com/items?name={{vars.itemName}}'
  expect:
    code: OK
    body:
      name: '{{vars.itemName}}'
  bind:
    vars:
      itemId: '{{response.body.id}}'
- title: create
  if: '{{steps.find.result == "failed"}}' # this step will be executed when the find step fails only
  protocol: http
  request:
    method: POST
    url: 'http://example.com/items'
    header:
      Content-Type: application/json
    body:
      name: '{{vars.itemName}}'
      price: '{{vars.itemPrice}}'
  expect:
    code: OK
    body:
      name: '{{vars.itemName}}'
  bind:
    vars:
      itemId: '{{response.body.id}}'
```

## Template String

Scenarigo provides the original template string feature which is evaluated at runtime. You can use expressions with a pair of double braces `{{}}` in YAML strings. All expression return an arbitrary value.

For instance, `'{{1}}'` is evaluated as an integer `1` at runtime.

```yaml
vars:
  id: '{{1}}' # id: 1
```

You can mix the templates into a raw string if all expressions' results are a string.

```yaml
vars:
  text: 'foo-{{"bar"}}-baz' # text: 'foo-bar-baz'
```

### Syntax

The grammar of the template is defined below, using `|` for alternatives, `[]` for optional, `{}` for repeated, `()` for grouping, and `...` for character range.

```
ParameterExpr   = "{{" Expr "}}"
Expr            = UnaryExpr | BinaryExpr | ConditionalExpr
UnaryExpr       = [UnaryOp] (
                    ParenExpr | SelectorExpr | IndexExpr | CallExpr |
                    INT | FLOAT | BOOL | STRING | IDENT
                  )
UnaryOp         = "!" | "-"
ParenExpr       = "(" Expr ")"
SelectorExpr    = Expr "." IDENT
IndexExpr       = Expr "[" INT "]"
CallExpr        = Expr "(" [Expr {"," Expr}] ")"
BinaryExpr      = Expr BinaryOp Expr
BinaryOp        = "+" | "-" | "*" | "/" | "%" |
                  "&&" | "||" |
                  "==" | "!=" | "<" | "<=" | ">" | ">=" 
ConditionalExpr = Expr ? Expr : Expr
```

The lexis is defined below.

```
INT           = "0" | ("1"..."9" {DECIMAL_DIGIT})
FLOAT         = INT "." DECIMAL_DIGIT {DECIMAL_DIGIT}
BOOL          = "true" | "false"
STRING        = `"` {UNICODE_VALUE} `"`
IDENT         = (LETTER {LETTER | DECIMAL_DIGIT | "-" | "_"} | "$") - RESERVED

DECIMAL_DIGIT = "0"..."9"
UNICODE_VALUE = UNICODE_CHAR | ESCAPED_CHAR
UNICODE_CHAR  = /* an arbitrary UTF-8 encoded char */
ESCAPED_CHAR  = "\" `"`
LETTER        = "a"..."Z"
TYPES         = "int" | "uint" | "float" | "bool" | "string" |
                "bytes" | "time" | "duration" | "any"
RESERVED      = BOOL | TYPES | "type" | "defined" | "size"
```

### Types

The template feature has abstract types for operations.

|Template Type|Description|Go Type|
|---|---|---|
|int|64-bit signed integers|int, int8, int16, int32, int64|
|uint|64-bit unsigned integers|uint, uint8, uint16, uint32, uint64|
|float|IEEE-754 64-bit floating-point numbers|float32, float64|
|bool|booleans|bool|
|string|UTF-8 strings|string|
|bytes|byte sequence|[]byte|
|time|time with nanosecond precision|[time.Time](https://pkg.go.dev/time#Time)|
|duration|amount of time|[time.Duration](https://pkg.go.dev/time#Duration)|
|any|other all Go types|any|

#### Type Conversions

The template feature provides functions to convert types.

<table>
  <thead>
    <tr>
      <th>Function</th>
      <th>Type</th>
      <th>Description</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td align="center" rowspan=5>int</td>
      <td>(*int) -> int</td>
      <td>type conversion (returns an error if arg is nil)</td>
    </tr>
    <tr>
      <td>(uint) -> int</td>
      <td>type conversion (returns an error if result is out of range)</td>
    </tr>
    <tr>
      <td>(float) -> int</td>
      <td>type conversion (rounds toward zero, returns an error if result is out of range)</td>
    </tr>
    <tr>
      <td>(string) -> int</td>
      <td>type conversion (returns an error if arg in invalid int string)</td>
    </tr>
    <tr>
      <td>(duration) -> int</td>
      <td>type conversion</td>
    </tr>
    <tr>
      <td align="center" rowspan=4>uint</td>
      <td>(int) -> uint</td>
      <td>type conversion (returns an error if result is out of range)</td>
    </tr>
    <tr>
      <td>(*uint) -> uint</td>
      <td>type conversion (returns an error if arg is nil)</td>
    </tr>
    <tr>
      <td>(float) -> uint</td>
      <td>type conversion (rounds toward zero, returns an error if result is out of range)</td>
    </tr>
    <tr>
      <td>(string) -> uint</td>
      <td>type conversion (returns an error if arg in invalid uint string)</td>
    </tr>
    <tr>
      <td align="center" rowspan=4>float</td>
      <td>(int) -> float</td>
      <td>type conversion</td>
    </tr>
    <tr>
      <td>(uint) -> float</td>
      <td>type conversion</td>
    </tr>
    <tr>
      <td>(*float) -> float</td>
      <td>type conversion (returns an error if arg is nil)</td>
    </tr>
    <tr>
      <td>(string) -> float</td>
      <td>type conversion (returns an error if arg in invalid float string)</td>
    </tr>
    <tr>
      <td align="center">bool</td>
      <td>(*bool) -> bool</td>
      <td>type conversion (returns an error if arg is nil)</td>
    </tr>
    <tr>
      <td align="center" rowspan=7>string</td>
      <td>(int) -> string</td>
      <td>type conversion</td>
    </tr>
    <tr>
      <td>(uint) -> string</td>
      <td>type conversion</td>
    </tr>
    <tr>
      <td>(float) -> string</td>
      <td>type conversion</td>
    </tr>
    <tr>
      <td>(*string) -> string</td>
      <td>type conversion (returns an error if arg is nil)</td>
    </tr>
    <tr>
      <td>(bytes) -> string</td>
      <td>type conversion (returns an error if arg contains invalid UTF-8 encoded characters)</td>
    </tr>
    <tr>
      <td>(time) -> string</td>
      <td>convert to string according to RFC3339 format</td>
    </tr>
    <tr>
      <td>(duration) -> string</td>
      <td>convert to string according to <a href="https://pkg.go.dev/time#Duration.String">time.Duration.String</a> format</td>
    </tr>
    <tr>
      <td align="center" rowspan=2>bytes</td>
      <td>(string) -> bytes</td>
      <td>type conversion</td>
    </tr>
    <tr>
      <td>(*bytes) -> bytes</td>
      <td>type conversion (returns an error if arg is nil)</td>
    </tr>
    <tr>
      <td align="center" rowspan=2>time</td>
      <td>(string) -> time</td>
      <td>parse RFC3339 format string as time</td>
    </tr>
    <tr>
      <td>(*time) -> time</td>
      <td>type conversion (returns an error if arg is nil)</td>
    </tr>
    <tr>
      <td align="center" rowspan=3>duration</td>
      <td>(int) -> duration</td>
      <td>type conversion</td>
    </tr>
    <tr>
      <td>(string) -> duration</td>
      <td>parse string as duration by <a href="https://pkg.go.dev/time#ParseDuration">time.ParseDuration</a></td>
    </tr>
    <tr>
      <td>(*duration) -> duration</td>
      <td>type conversion (returns an error if arg is nil)</td>
    </tr>
  </tbody>
</table>

### Operators

<table>
  <thead>
    <tr>
      <th>Operator</th>
      <th>Type</th>
      <th>Description</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td align="center">! _</td>
      <td>(bool) -> bool</td>
      <td>logical not</td>
    </tr>
    <tr>
      <td align="center" rowspan=3>- _</td>
      <td>(int) -> int</td>
      <td>negation</td>
    </tr>
    <tr>
      <td>(float) -> float</td>
      <td>negation</td>
    </tr>
    <tr>
      <td>(duration) -> duration</td>
      <td>negation</td>
    </tr>
    <tr>
      <td align="center" rowspan=8>_ + _</td>
      <td>(int, int) -> int</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(uint, uint) -> uint</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(float, float) -> float</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(string, string) -> string</td>
      <td>concatenation</td>
    </tr>
    <tr>
      <td>(bytes, bytes) -> bytes</td>
      <td>concatenation</td>
    </tr>
    <tr>
      <td>(time, duration) -> time</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(duration, time) -> time</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(duration, duration) -> duration</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td align="center" rowspan=6>_ - _</td>
      <td>(int, int) -> int</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(uint, uint) -> uint</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(float, float) -> float</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(time, time) -> duration</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(time, duration) -> time</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(duration, duration) -> duration</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td align="center" rowspan=3>_ * _</td>
      <td>(int, int) -> int</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(uint, uint) -> uint</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(float, float) -> float</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td align="center" rowspan=3>_ / _</td>
      <td>(int, int) -> int</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(uint, uint) -> uint</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(float, float) -> float</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td align="center" rowspan=2>_ % _</td>
      <td>(int, int) -> int</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td>(uint, uint) -> uint</td>
      <td>arithmetic</td>
    </tr>
    <tr>
      <td align="center">_ == _</td>
      <td>(A, A) -> bool</td>
      <td>equality</td>
    </tr>
    <tr>
      <td align="center">_ != _</td>
      <td>(A, A) -> bool</td>
      <td>inequality</td>
    </tr>
    <tr>
      <td align="center" rowspan=7>_ < _</td>
      <td>(int, int) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(uint, uint) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(float, float) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(string, string) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(bytes, bytes) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(time, time) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(duration, duration) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td align="center" rowspan=7>_ <= _</td>
      <td>(int, int) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(uint, uint) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(float, float) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(string, string) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(bytes, bytes) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(time, time) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(duration, duration) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td align="center" rowspan=7>_ > _</td>
      <td>(int, int) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(uint, uint) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(float, float) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(string, string) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(bytes, bytes) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(time, time) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(duration, duration) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td align="center" rowspan=7>_ >= _</td>
      <td>(int, int) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(uint, uint) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(float, float) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(string, string) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(bytes, bytes) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(time, time) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td>(duration, duration) -> bool</td>
      <td>ordering</td>
    </tr>
    <tr>
      <td align="center">_ && _</td>
      <td>(bool, bool) -> bool</td>
      <td>logical and</td>
    </tr>
    <tr>
      <td align="center">_ || _</td>
      <td>(bool, bool) -> bool</td>
      <td>logical or</td>
    </tr>
    <tr>
      <td align="center">_ ? _ : _</td>
      <td>(bool, A, A) -> A</td>
      <td>ternary conditional operator</td>
    </tr>
  </tbody>
</table>

### Predefined Variables

|Variables|Description|
|---|---|
|vars|user-defined variables|
|plugins|loaded plugins|
|env|environment variables|
|request|request data|
|response|response data|
|assert|assert functions|
|steps|results of steps|

### Predefined Functions

<table>
  <thead>
    <tr>
      <th>Function</th>
      <th>Description</th>
      <th>Example</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td>type</td>
      <td>returns the abstract type of expression in string</td>
      <td><code>type(0) == "int"</code></td>
    </tr>
    <tr>
      <td>defined</td>
      <td>tells whether a variable is defined or not</td>
      <td><code>defined(a) ? a : b</code></td>
    </tr>
    <tr>
      <td rowspan=4>size</td>
      <td>returns the string length</td>
      <td><code>size("foo")</code></td>
    </tr>
    <tr>
      <td>returns the bytes length</td>
      <td><code>size(bytes("foo"))</code></td>
    </tr>
    <tr>
      <td>returns the number of list elements</td>
      <td><code>size(items)</code></td>
    </tr>
    <tr>
      <td>returns the number of map elements</td>
      <td><code>size(index)</code></td>
    </tr>
  </tbody>
</table>

## Plugin

Scenarigo has a plugin mechanism that enables you to add new functionalities you need by writing Go code.
This feature is based on [Go's standard library `plugin`](https://pkg.go.dev/plugin), which has the following limitations.

- Supported on Linux, FreeBSD, and macOS only.
- All plugins (and installed `scenarigo` command) must be built with the same version of the Go compiler and dependent packages.

Scenarigo loads built plugins at runtime and accesses any exported variable or function via [template string](#template-string).

See [the official document](https://pkg.go.dev/plugin) for details of the `plugin` package.

### How to write plugins

A Go plugin is a `main` package with **exported** variables and functions.

```go main.go
package main

import "time"

var Layout = "2006-01-02"

func Today() string {
	return time.Now().Format(Layout)
}

```

You can use the variables and functions via template strings like below in your test scenarios.

- `{{plugins.date.Layout}}` => `"2006-01-02"`
- `{{plugins.date.Today()}}` => `"2022-02-22"`

Scenarigo allows functions to return a value or a value and an error. The template string execution will fail if the function returns a non-nil error.


```go main.go
package main

import "time"

var Layout = "2006-01-02"

func TodayIn(s string) (string, error) {
	loc, err := time.LoadLocation(s)
	if err != nil {
		return "", err
	}
	return time.Now().In(loc).Format(Layout), nil
}
```

- `{{plugins.date.TodayIn("UTC")}}` => `"2022-02-22"`
- `{{plugins.date.TodayIn("INVALID")}}` => `failed to execute: {{plugins.date.TodayIn("INVALID")}}: unknown time zone INVALID`

### How to build plugins

Go plugin can be built with `go build -buildmode=plugin`, but we recommend you use `scenarigo plugin build` instead. The wrapper command requires `go` command installed in your machine.

Scenarigo builds plugins according to the configuration.

```yaml scenarigo.yaml
schemaVersion: config/v1

scenarios:
- scenarios

pluginDirectory: ./gen  # Specify the root directory of plugins.
plugins:                # Specify configurations to build plugins.
  date.so:              # Map keys specify plugin output file path from the root directory of plugins.
    src: ./plugins/date # Specify the source file, directory, or "go gettable" module path of the plugin.
```

```shell
.
├── plugins
│   └── date
│       └── main.go
├── scenarigo.yaml
└── scenarios
    └── echo.yaml
```

In this case, the plugin will be built and written to `date.so`.

```shell
$ scenarigo plugin build
```

```shell
.
├── gen
│   └── date.so     # built plugin
├── plugins
│   └── date
│       ├── go.mod  # generated automatically if not exists
│       └── main.go
├── scenarigo.yaml
└── scenarios
    └── echo.yaml
```

Scenarigo checks the dependent packages of each plugin before building. If the plugins depend on a different version of the same package, Scenarigo overrides `go.mod` files by the maximum version to avoid the build error.

Now you can use the plugin in test scenarios.

```yaml echo.yaml
title: echo
plugins:
  date: date.so # relative path from "pluginDirectory"
steps:
- title: POST /echo
  protocol: http
  request:
    method: POST
    url: 'http://{{env.ECHO_ADDR}}/echo'
    body:
      message: '{{plugins.date.Today()}}'
  expect:
    code: 200
```

Scenarigo can download source codes from remote repositories and build it with [`go get`-able](https://go.dev/ref/mod#go-get) module query.

```yaml scenarigo.yaml
plugins:
  uuid.so:
    src: github.com/zoncoen-sample/scenarigo-plugins/uuid@latest
```

### Advanced features

#### Setup Funciton

[`plugin.RegisterSetup`](https://pkg.go.dev/github.com/zoncoen/scenarigo/plugin#RegisterSetup) registers a setup function that will be called before running scenario tests once only. If the registered function returns a non-nil function as a second returned value, it will be executed after finished all tests.

```go main.go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/zoncoen/scenarigo/plugin"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

const (
	projectName = "foo"
)

func init() {
	plugin.RegisterSetup(setupClient)
}

var client *secretmanager.Client

func setupClient(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	var err error
	client, err = secretmanager.NewClient(context.Background())
	if err != nil {
		ctx.Reporter().Fatalf("failed to create secretmanager client: %v", err)
	}
	return ctx, func(ctx *plugin.Context) {
		client.Close()
	}
}

func GetSecretString(name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", projectName, name),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get secret: %v", err)
	}
	return string(resp.Payload.Data), nil
}
```

```yaml scenarigo.yaml
plugins:
  setup.so:
    src: ./plugins/date # call "setupClient" before running test scenarios
```

Similarly, [`plugin.RegisterSetupEachScenario`](https://pkg.go.dev/github.com/zoncoen/scenarigo/plugin#RegisterSetupEachScenario) can register a setup function. The registered function will be called before each test scenario that uses the plugin.

```go main.go
package main

import (
	"github.com/zoncoen/scenarigo/plugin"

	"github.com/google/uuid"
)

func init() {
	plugin.RegisterSetupEachScenario(setRunID)
}

func setRunID(ctx *plugin.Context) (*plugin.Context, func(*plugin.Context)) {
	return ctx.WithVars(map[string]string{
		"runId": uuid.NewString(),
	}), nil
}
```

```yaml echo.yaml
title: echo
plugins:
  setup: setup.so # call "setRunID" before running this test scenario
steps:
- title: POST /echo
  protocol: http
  request:
    method: POST
    url: 'http://{{env.ECHO_ADDR}}/echo'
    header:
      Run-Id: '{{vars.runId}}'
    body:
      message: hello
  expect:
    code: 200
```

#### Custom Step Function

Generally, a `step` represents sending a request in Scenarigo. However, you can use a Go's function as a step with the plugin.

```go main.go
package main

import (
	"github.com/zoncoen/scenarigo/plugin"
	"github.com/zoncoen/scenarigo/schema"
)

var Nop = plugin.StepFunc(func(ctx *plugin.Context, step *schema.Step) *plugin.Context {
	ctx.Reporter().Log("nop step")
	return ctx
})
```

```yaml nop.yaml
title: nop
plugins:
  step: step.so
steps:
- title: nop step
  ref: '{{plugins.step.Nop}}'
```

#### Left Arrow Function (a function takes arguments in YAML)

Scenarigo enables you to define a function that takes arguments in YAML for readability. It is called the "Left Arrow Function" since its syntax `{{funcName <-}}`.

```go main.go
package main

import (
	"errors"
	"fmt"

	"github.com/zoncoen/scenarigo/plugin"
)

var CoolFunc plugin.LeftArrowFunc = &fn{}

type fn struct{}

type arg struct {
	Foo string `yaml:"foo"`
	Bar string `yaml:"bar"`
	Baz string `yaml:"baz"`
}

func (_ *fn) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
	var a arg
	if err := unmarshal(&a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (_ *fn) Exec(in interface{}) (interface{}, error) {
	a, ok := in.(*arg)
	if !ok {
		return nil, errors.New("arg must be a arg")
	}
	return fmt.Sprintf("foo: %s, bar: %s, baz: %s", a.Foo, a.Bar, a.Baz), nil
}
```

```yaml echo.yaml
title: echo
plugins:
  cool: cool.so
steps:
- title: POST /echo
  protocol: http
  request:
    method: POST
    url: 'http://{{env.ECHO_ADDR}}/echo'
    body:
      message:
        '{{plugins.cool.CoolFunc <-}}':
          foo: 1
          bar: 2
          baz: 3
  expect:
    code: 200
```

## ytt Integration (templating and overlays)

Scenarigo integrates [ytt](https://carvel.dev/ytt/) to provide flexible templating and overlay features for test scenarios. You can use this experimental feature by enabling it in `scenarigo.yaml`.

```yaml scenarigo.yaml
input:
  yaml:
    ytt:
      enabled: true
```

### Single File

All test scenarios are processed as ytt templates when the feature is enabled. For example, the following simple test scenario will set "hello" to the `message` field.

```yaml simple.yaml
#@ msg = "hello"
schemaVersion: scenario/v1
title: echo
steps:
- title: POST /echo
  protocol: http
  request:
    method: POST
    url: http://example.com/echo
    header:
      Content-Type: application/json
    body:
      message: #@ msg
  expect:
    body:
      message: "{{request.body.message}}"
```

You can check the test scenarios generated by ytt integration with `scenarigo dump` sub-command.

```shell
$ scenarigo dump ./scenarios/simple.yaml
schemaVersion: scenario/v1
title: echo
steps:
- title: POST /echo
  protocol: http
  request:
    method: POST
    url: http://example.com/echo
    header:
      Content-Type: application/json
    body:
      message: hello
  expect:
    body:
      message: "{{request.body.message}}"
```

### Multiple File

`ytt/v1` schema type file allows giving multiple ytt files.

```yaml scenarios.yaml
# This configuration equals the following command.
# ytt -f template.ytt.yaml -f values.ytt.yaml
schemaVersion: ytt/v1
files:
- template.ytt.yaml
- values.ytt.yaml
```

```yaml template.ytt.yaml
#@ load("@ytt:data", "data")
#@ for params in data.values:
---
schemaVersion: scenario/v1
plugins:
  plugin: plugin.so
title: #@ params.title
vars: #@ params.vars
steps:
- title: #@ "{} /{}".format(params.request.method, params.request.path)
  protocol: http
  request:
    method: #@ params.request.method
    url: #@ "http://example.com/{}".format(params.request.path)
    header:
      Content-Type: application/json
    body:
      message: "{{vars.message}}"
  expect: #@ params.expect
#@ end
```

```yaml values.ytt.yaml
#@data/values
---
- title: success
  vars:
    message: hello
  request:
    method: POST
    path: echo
  expect:
    code: OK
    body:
      message: "{{request.body.message}}"

- title: invalid method
  vars:
    message: hello
  request:
    method: GET
    path: echo
  expect:
    code: Method Not Allowed

- title: invalid path
  vars:
    message: hello
  request:
    method: POST
    path: invalid
  expect:
    code: Not Found
```

This example will run three test scenarios.

```shell
$ scenarigo dump ./scenarios/scenarios.yaml
schemaVersion: scenario/v1
title: success
plugins:
  plugin: plugin.so
vars:
  message: hello
steps:
- title: POST /echo
  protocol: http
  request:
    method: POST
    url: http://example.com/echo
    header:
      Content-Type: application/json
    body:
      message: "{{vars.message}}"
  expect:
    code: OK
    body:
      message: "{{request.body.message}}"
---
schemaVersion: scenario/v1
title: invalid method
plugins:
  plugin: plugin.so
vars:
  message: hello
steps:
- title: GET /echo
  protocol: http
  request:
    method: GET
    url: http://example.com/echo
    header:
      Content-Type: application/json
    body:
      message: "{{vars.message}}"
  expect:
    code: Method Not Allowed
---
schemaVersion: scenario/v1
title: invalid path
plugins:
  plugin: plugin.so
vars:
  message: hello
steps:
- title: POST /invalid
  protocol: http
  request:
    method: POST
    url: http://example.com/invalid
    header:
      Content-Type: application/json
    body:
      message: "{{vars.message}}"
  expect:
    code: Not Found
```

:warning: You should exclude ytt files specified from `ytt/v1` type test scenarios by setting [regular expressions](https://pkg.go.dev/regexp/syntax#hdr-Syntax) to `excludes` field.

```yaml scenarigo.yaml
input:
  excludes:
  - \.ytt\.yaml$
  yaml:
    ytt:
      enabled: true
```

### Default ytt Files

The files set to `defaultFiles` field will be used to generate all test scenarios.

```yaml scenarigo.yaml
input:
  yaml:
    ytt:
      enabled: true
      defaultFiles:
      - default.yaml
```

```yaml default.yaml
#@ load("@ytt:overlay", "overlay")
#@overlay/match by=overlay.map_key("schemaVersion"), expects="0+"
---
schemaVersion: scenario/v1
steps:
#@overlay/match by=overlay.all, expects="0+"
-
  #@overlay/match when=0
  timeout: 30s
```

This example set 30 sec. as the default timeout for all test scenarios.

```shell
$ scenarigo dump ./scenarios/simple.yaml
schemaVersion: scenario/v1
title: echo
plugins:
  plugin: plugin.so
steps:
- title: POST /echo
  protocol: http
  request:
    method: POST
    url: http://{{plugins.plugin.ServerAddr}}/echo
    header:
      Content-Type: application/json
    body:
      message: hello
  expect:
    body:
      message: "{{request.body.message}}"
  timeout: 30s
```
