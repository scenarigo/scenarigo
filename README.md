# Scenarigo

[![godoc](https://godoc.org/github.com/zoncoen/scenarigo?status.svg)](https://pkg.go.dev/github.com/zoncoen/scenarigo)
![test](https://github.com/zoncoen/scenarigo/workflows/test/badge.svg?branch=master)
[![codecov](https://codecov.io/gh/zoncoen/scenarigo/branch/master/graph/badge.svg)](https://codecov.io/gh/zoncoen/scenarigo)
[![go report](https://goreportcard.com/badge/zoncoen/scenarigo)](https://goreportcard.com/report/github.com/zoncoen/scenarigo)
[![codebeat](https://codebeat.co/badges/93ee2453-1a25-4db6-b98e-c430c994b4b8)](https://codebeat.co/projects/github-com-zoncoen-scenarigo-master)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

A scenario-based API testing tool for HTTP/gRPC server.

## Overview

Scenarigo is a scenario-based API testing tool for HTTP/gRPC server.
It is written in Go, enable to customize by [the plugin package of Go](https://golang.org/pkg/plugin/).
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

### go install command

```shell
$ go install github.com/zoncoen/scenarigo/cmd/scenarigo@v0.8.1
```

### from release page

Go to the [releases page](https://github.com/zoncoen/scenarigo/releases) and download the zip file. Unpack the zip file, and put the binary to a directory in your `$PATH`.

You can download the latest command into the `./scenarigo` directory with the following one-liner code. Place the binary `./scenarigo/scenarigo` into your `$PATH`.

```shell
$ version=$(curl -s https://api.github.com/repos/zoncoen/scenarigo/releases/latest | jq -r '.tag_name') && \
    go_version='go1.17' && \
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

scenarios: []       # Specify test scenario files and directories.
pluginDirectory: ./ # Specify the root directory of plugins.

output:
  verbose: false          # Enable verbose output.
  # colored: false        # Enable colored output with ANSI color escape codes. It is enabled by default but disabled when a NO_COLOR environment variable is set (regardless of its value).
  # report:
  #   json:
  #     filename: ./report.json # Specify a filename for test report output in JSON.
  #   junit:
  #     filename: ./junit.xml # Specify a filename for test report output in JUnit XML format.
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
  config      manage the scenarigo configuration file
  help        Help about any command
  list        list the test scenarios
  run         run test scenarios
  version     print scenarigo version

Flags:
  -c, --config string   specify configuration file path
  -h, --help            help for scenarigo

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

### Plugin

Scenarigo provides the extension feature by using [the plugin package of Go](https://golang.org/pkg/plugin/). It enables to use Go functions and variables in test scenarios. For example, you can use UUID in scenario yaml file.

First, create plugin file in the plugins dir which is specified at `pluginDirectory` in scenarigo.yaml.

```go gen.go
package main
import (
    "github.com/google/uuid"
)
func UUID() string{
    return uuid.New().String()
}
```

Second, build the plugin file with `buildmode=plugin` option.

**Notes**: You have to build the plugin with the same version of Go as the scnarigo run command.

```shell
$ go build -buildmode=plugin -o gen.so gen.go
```

Then, you can load the plugin `gen.so` and use `UUID()` function in the scenario yaml file.

```yaml
title: check /message
plugins: # Specify Go plugin files
  gen: gen.so
steps:
- title: POST /message
  protocol: http
  request:
    method: POST
    url: http://example.com/message
    body:
      id: '{{plugins.gen.UUID()}}' # Call Go plugin function
```
