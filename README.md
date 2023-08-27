<a href="https://github.com/zoncoen/scenarigo">
  <img alt="Scenarigo" src="https://user-images.githubusercontent.com/2238852/205980597-37eaaf03-fd35-4a04-93c4-884c95f48df3.png" width="485px">
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

## Installation

### go install command (recommend)

```shell
$ go install github.com/zoncoen/scenarigo/cmd/scenarigo@v0.14.2
```

### from release page

Go to the [releases page](https://github.com/zoncoen/scenarigo/releases) and download the zip file. Unpack the zip file, and put the binary to a directory in your `$PATH`.

You can download the latest command into the `./scenarigo` directory with the following one-liner code. Place the binary `./scenarigo/scenarigo` into your `$PATH`.

```shell
$ version=$(curl -s https://api.github.com/repos/zoncoen/scenarigo/releases/latest | jq -r '.tag_name') && \
    go_version='go1.21.0' && \
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

scenarios: [] # Specify test scenario files and directories.

pluginDirectory: ./gen    # Specify the root directory of plugins.
plugins:                  # Specify configurations to build plugins.
  plugin.so:              # Map keys specify plugin output file path from the root directory of plugins.
    src: ./path/to/plugin # Specify the source file, directory, or "go gettable" module path of the plugin.

output:
  verbose: false # Enable verbose output.
  colored: false # Enable colored output with ANSI color escape codes. It is enabled by default but disabled when a NO_COLOR environment variable is set (regardless of its value).
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

|retry #|retry interval|randomized interval range|
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

### Template string

Scenarigo provides the original template string feature. It enables to store and reuse values in test scenarios.
The `vars` field defines variables that can be referred by template string like `'{{vars.id}}'`.

```yaml
title: check /message
vars:
  id: 1
steps:
- title: GET /message
  protocol: http
  request:
    method: GET
    url: http://example.com/message
    query:
      id: '{{vars.id}}'
```

You can define "step" scope variables that can't be accessed from other steps.

```yaml
title: check /message
steps:
- title: GET /message
  vars:
  - 1
  protocol: http
  request:
    method: GET
    url: http://example.com/message
    query:
      id: '{{vars[0]}}'
```

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
