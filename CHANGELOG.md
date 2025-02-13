# CHANGELOG

## [v0.19.0](https://github.com/scenarigo/scenarigo/compare/v0.18.0...v0.19.0) - 2025-02-12
### New Features
- Introduce nullish coalescing operator by @youxkei in https://github.com/scenarigo/scenarigo/pull/495
### Dependency Upgrades
- chore(deps): bump github.com/goccy/go-yaml from 1.15.15 to 1.15.17 by @dependabot in https://github.com/scenarigo/scenarigo/pull/505
- chore(deps): bump golang.org/x/text from 0.21.0 to 0.22.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/507
- chore(deps): bump golang.org/x/mod from 0.22.0 to 0.23.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/508
- chore(deps): bump google.golang.org/protobuf from 1.36.4 to 1.36.5 by @dependabot in https://github.com/scenarigo/scenarigo/pull/509
- chore(deps): bump github.com/goccy/go-yaml from 1.15.17 to 1.15.22 by @dependabot in https://github.com/scenarigo/scenarigo/pull/516

## [v0.18.0](https://github.com/scenarigo/scenarigo/compare/v0.17.3...v0.18.0) - 2025-01-29
### New Features
- add secrets field to define credential variables by @zoncoen in https://github.com/scenarigo/scenarigo/pull/414
- always update toolchain directive to build plugins with the go version used to build scenarigo by @zoncoen in https://github.com/scenarigo/scenarigo/pull/421
- add verbose flag to "plugin" sub-command by @zoncoen in https://github.com/scenarigo/scenarigo/pull/454
- enable to create protocol buffer messages dynamically from proto files by @zoncoen in https://github.com/scenarigo/scenarigo/pull/457
- add protocol option by @zoncoen in https://github.com/scenarigo/scenarigo/pull/477
- enable to use go.work for building plugins by @zoncoen in https://github.com/scenarigo/scenarigo/pull/499
### Bug Fixes
- fix: keep orders of protocol options by @zoncoen in https://github.com/scenarigo/scenarigo/pull/480
- Fix parsing of template contains new line char by @goccy in https://github.com/scenarigo/scenarigo/pull/500
### Dependency Upgrades
- chore(deps): bump google.golang.org/grpc from 1.63.0 to 1.63.2 by @dependabot in https://github.com/scenarigo/scenarigo/pull/412
- chore(deps): bump golang.org/x/net from 0.21.0 to 0.23.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/415
- chore(deps): bump golang.org/x/text from 0.14.0 to 0.15.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/417
- chore(deps): bump google.golang.org/protobuf from 1.33.0 to 1.34.1 by @dependabot in https://github.com/scenarigo/scenarigo/pull/418
- chore(deps): bump github.com/fatih/color from 1.16.0 to 1.17.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/419
- chore(deps): bump google.golang.org/grpc from 1.63.2 to 1.64.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/420
- chore(deps): bump golang.org/x/text from 0.15.0 to 0.16.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/424
- chore(deps): bump golang.org/x/mod from 0.17.0 to 0.18.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/423
- chore(deps): bump google.golang.org/protobuf from 1.34.1 to 1.34.2 by @dependabot in https://github.com/scenarigo/scenarigo/pull/425
- chore(deps): bump golang.ort/x/* to the latest by @zoncoen in https://github.com/scenarigo/scenarigo/pull/428
- chore(deps): bump github.com/spf13/cobra from 1.8.0 to 1.8.1 by @dependabot in https://github.com/scenarigo/scenarigo/pull/430
- chore(deps): bump github.com/vektah/gqlparser/v2 from 2.5.1 to 2.5.14 in /scripts/cross-build by @dependabot in https://github.com/scenarigo/scenarigo/pull/426
- chore(deps): bump indirect modules by @zoncoen in https://github.com/scenarigo/scenarigo/pull/437
- chore(deps): bump github.com/goccy/go-yaml from 1.11.3 to 1.12.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/433
- chore(deps): bump golang.org/x/mod from 0.18.0 to 0.20.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/435
- chore(deps): bump golang.org/x/sync from 0.7.0 to 0.8.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/436
- chore(deps): bump actions/checkout from 3 to 4 by @dependabot in https://github.com/scenarigo/scenarigo/pull/441
- chore(deps): bump google.golang.org/grpc from 1.64.0 to 1.65.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/431
- chore(deps): bump golang.org/x/text from 0.16.0 to 0.17.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/442
- chore(deps): bump google.golang.org/grpc from 1.65.0 to 1.66.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/445
- chore(deps): bump golang.org/x/mod from 0.20.0 to 0.21.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/447
- chore(deps): bump golang.org/x/text from 0.17.0 to 0.19.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/451
- chore(deps): bump google.golang.org/grpc from 1.66.0 to 1.67.1 by @dependabot in https://github.com/scenarigo/scenarigo/pull/456
- chore(deps): bump google.golang.org/protobuf from 1.34.2 to 1.35.1 by @dependabot in https://github.com/scenarigo/scenarigo/pull/452
- chore(deps): bump github.com/fatih/color from 1.17.0 to 1.18.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/453
- Update goccy/go-yaml to v1.15.12 by @goccy in https://github.com/scenarigo/scenarigo/pull/476
- chore(deps): bump github.com/jhump/protoreflect from 1.16.0 to 1.17.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/469
- chore(deps): bump golang.org/x modules by @zoncoen in https://github.com/scenarigo/scenarigo/pull/478
- chore(deps): bump codecov/codecov-action from 4 to 5 by @dependabot in https://github.com/scenarigo/scenarigo/pull/465
- chore(deps): bump goreleaser/goreleaser-action from 5 to 6 by @dependabot in https://github.com/scenarigo/scenarigo/pull/422
- chore(deps): bump github.com/goccy/go-yaml from 1.15.12 to 1.15.13 by @dependabot in https://github.com/scenarigo/scenarigo/pull/484
- chore(deps): bump google.golang.org/protobuf from 1.36.0 to 1.36.1 by @dependabot in https://github.com/scenarigo/scenarigo/pull/485
- chore(deps): bump google.golang.org/grpc from 1.67.1 to 1.69.2 by @dependabot in https://github.com/scenarigo/scenarigo/pull/479
- chore(deps): bump google.golang.org/grpc from 1.69.2 to 1.70.0 by @dependabot in https://github.com/scenarigo/scenarigo/pull/496
- chore(deps): bump google.golang.org/protobuf from 1.36.1 to 1.36.4 by @dependabot in https://github.com/scenarigo/scenarigo/pull/497
- chore(deps): bump github.com/goccy/go-yaml from 1.15.13 to 1.15.15 by @dependabot in https://github.com/scenarigo/scenarigo/pull/491

## [v0.17.3] - 2024-04-14
### Bug Fixes
- **release:** ensure the Docker image exists ([#411](https://github.com/scenarigo/scenarigo/issues/411))

## [v0.17.2] - 2024-03-21
### Features
- add an option to output test summary at last ([#395](https://github.com/scenarigo/scenarigo/issues/395))

## [v0.17.1] - 2024-02-26
### Bug Fixes
- **assert:** fix retry failure if using left arrow functions for assertion
- **assert:** show the correct error position even when using left arrow functions
- **assert:** don't wrap nil error to fix notContains

### Features
- bump the minimum go version
- **plugin:** set GOTOOLCHAIN env for building plugins

## [v0.17.0] - 2024-01-26
### Bug Fixes
- **assert:** fix errors when using assert.(and|or)
- **template:** convert into string in implicit concatenation

### Features
- **errors:** allow adding path index
- **grpc:** enable querying by protobuf struct tag
- **grpc:** enable to access metadata etc. via the request/response variable
- **http:** enable to access header etc. via the request/response variable

### BREAKING CHANGE

template.Executefunction requires a context.Context value as an argument to avoid a goroutine leak.

## [v0.16.2] - 2023-11-02
### Bug Fixes
- **template:** don't convert int to string

## [v0.16.1] - 2023-11-01
### Features
- **grpc:** dump invalid utf8 strings as hex

## [v0.16.0] - 2023-10-24
### Bug Fixes
- don't panic if the protocol is empty

### Features
- add continueOnError to prevent failure due to step errors
- add if field for controlling step running
- allow to access results of each step
- enable to assert by template string expressions
- cancel request contexts after each step
- **assert:** enable to pass custom equalers
- **config:** add global variables
- **template:** add size() function
- **template:** allow to call values having Call method as a function
- **template:** allow '$' identifier

### BREAKING CHANGE

assert.Build function requires a context.Context value as an argument to avoid a goroutine leak.

## [v0.15.1] - 2023-09-15
### Code Refactoring
- remove workaround

## [v0.15.0] - 2023-09-06
### Bug Fixes
- **plugin:** setup plugins in the order in which they are registered
- **schema:** add a workaround to avoid failing to load scenarios
- **template:** check overflow
- **template:** evaluate only an expression that matched the condition

### Code Refactoring
- add OrderedMap

### Features
- add dump sub-command
- add ytt integration
- add input config
- **grpc:** contain response status in log
- **template:** add time and duration type
- **template:** add bytes type

## [v0.14.2] - 2023-03-03
- bump up the version of dependent modules

## [v0.14.1] - 2023-02-27
### Features
- **schema:** add Comments field

## [v0.14.0] - 2023-02-20
### Bug Fixes
- pass bound variables to the next step
- fix to filter correctly even if / is included in subtest names
- filter test by -run flag of go test
- **plugin:** make RegisterSetup() not cause an error if called in tests

### Code Refactoring
- **reporter:** change FromT implementation

### Features
- change retry unit from request to entire step
- **http:** add Accept-Encoding header by default
- **http:** enable decoding of response bodies with character encodings other than utf-8
- **http:** add text/html unmarshaler

## [v0.13.2] - 2022-12-16
- bump up the version of dependent modules

## [v0.13.1] - 2022-12-15
- bump up the version of dependent modules

## [v0.13.0] - 2022-12-08
### Bug Fixes
- enable to specify report paths by absolute path
- fix generate CREDITS workflow

### Features
- enable to read config from stdin
- enable to marshal schema.Config to YAML
- **errors:** change error message format

## [v0.12.8] - 2022-10-18
### Bug Fixes
- don't bind vars if included scenario failed

## [v0.12.7] - 2022-09-27
### Features
- **template:** enable to call methods

## [v0.12.6] - 2022-09-13
### Features
- enable to specify step timeout
- **grpc:** enable to use template in error details
- **http:** make method name case-insensitive

## [v0.12.5] - 2022-08-22
### Bug Fixes
- **plugin:** go mod tidy with -compat option

## [v0.12.4] - 2022-07-25
### Bug Fixes
- **plugin:** enable to replace modules to local paths
- **plugin:** keep replace directives

## [v0.12.3] - 2022-07-21
### Bug Fixes
- **plugin:** remove plugin modules from the cache
- **plugin:** check remote module source versions

## [v0.12.2] - 2022-07-20
### Bug Fixes
- **mock:** fix nil error bug
- **plugin:** force all plugins to use the same version of package

### Code Refactoring
- fix maintidx error
- fix cyclop error

## [v0.12.1] - 2022-06-26
### Bug Fixes
- **release:** reduce target Go versions

## [v0.12.0] - 2022-06-13
### Bug Fixes
- **plugin:** suppress unnecessary plugin build logs
- **plugin:** don't use "main" as module name

### Features
- **template:** allow functions to return an error

## [v0.11.2] - 2022-04-26
### Bug Fixes
- **plugin:** allow specifying sub directories of remote modules as src

## [v0.11.1] - 2022-04-18
### Bug Fixes
- print error if fail to open plugin
- **doc:** setup field was deprecated

## [v0.11.0] - 2022-04-15
### Bug Fixes
- **plugin:** fix issue with plugin build failure in Go1.18

### Features
- enable to marshal scenarios into YAML
- **mock:** enable to assert request
- **template:** allow writing left arrow function call in map syntax
- **template:** enable to use template in map keys
- **template:** enable to escape { by \

## [v0.10.0] - 2022-01-31
### Bug Fixes
- update the go directive of go.mod
- **plugin:** use the same module version as scenarigo for building plugins

### BREAKING CHANGE

This package requires Go 1.17 or later.

## [v0.9.0] - 2021-12-03
### Bug Fixes
- **errors:** Errors returns nil if no errors

### Code Refactoring
- use yaml.PathBuilder to specify the pos

### Features
- add setup feature
- add "scenarigo plugin list" command
- add "scenarigo config validate" command
- add plugin sub-command
- **plugin:** enable registration of setup functions to be executed for each scenario
- **plugin:** enable to build plugin from remote "go gettable" src
- **template:** add bool literals

## [v0.8.1] - 2021-09-27
### Bug Fixes
- add workaround to avoid the bug of Go 1.17

### Code Refactoring
- export functions

### Features
- list command refers to the configuration file
- remove blank lines from logs

### BREAKING CHANGE

"file" and "verbose" options are removed from the list sub-command.

## [v0.8.0] - 2021-09-08
### Bug Fixes
- enable CGO on release build
- **query:** do not extract by the inline field name
- **template:** fix a bug by nil struct field
- **template:** marshal variables to YAML in LAF arguments
- **template:** keep the original memory address
- **template:** marshal LAF arguments with indent

### Features
- enable cross compile with CGO
- **grpc:** loose type checking for equaler
- **template:** execute templates of data
- **version:** get version from build info

## [v0.7.0] - 2021-07-30
### Bug Fixes
- **assert:** fix the assertion operators
- **assert:** fix the logic to compare Go protobuf APIv2 messages
- **grpc:** rename body field to message
- **query:** don't access unexported field

### Code Refactoring
- don't use ioutil package

### Features
- change default configuration filename
- enable to set configurations by file
- add WithConfig option
- colorize outputs
- support NO_COLOR standard
- enable strictly check on request field
- use Go protobuf APIv2
- **assert:** enable to change the behavior of equal assertion
- **query:** allow accessing anonymous fields

### Performance Improvements
- reuse parsed AST node to print error tokens

### BREAKING CHANGE

This package requires Go 1.16 or later.

## [v0.6.3] - 2021-04-08
### Bug Fixes
- enable to bind vars defined in the included scenario

## [v0.6.2] - 2021-04-07
### Bug Fixes
- **plugin:** avoid the error caused by loading plugins concurrently ([#78](https://github.com/scenarigo/scenarigo/issues/78))

### Code Refactoring
- **assert:** remove query from arguments

### Features
- **assert:** add length assertion
- **assert:** add greaterThan/greaterThanOrEqual/lessThan/lessThanOrEqual ([#77](https://github.com/scenarigo/scenarigo/issues/77))
- **reporter:** enable to generate test report ([#83](https://github.com/scenarigo/scenarigo/issues/83))
- **reporter:** include the execution time of sub-tests ([#82](https://github.com/scenarigo/scenarigo/issues/82))

## [v0.6.1] - 2021-01-14
### Bug Fixes
- **template:** don't convert invalid values to avoid panic

## [v0.6.0] - 2021-01-12
### Bug Fixes
- **template:** enable to set to pointer values

### Features
- export RunScenario function
- add WithScenariosFromReader option
- allow template in header assertion
- **assert:** add regexp function
- **context:** add ScenarioFilePath

## [v0.5.1] - 2020-10-23
### Bug Fixes
- **template:** restore funcs in args of left arrow function

### Features
- **assert:** add "and" function

## [v0.5.0] - 2020-10-05
### Features
- **assert:** add "or" function
- **expect:** enable strict option when decoding yaml for expect to prevent field misplacement ([#59](https://github.com/scenarigo/scenarigo/issues/59))
- **grpc:** allow using a template as code and msg
- **http:** allow using a template as code

## [v0.4.0] - 2020-09-02
### Bug Fixes
- register errdetails proto messages to unmarshal Any
- **expect:** use the default assertion if no expect ([#55](https://github.com/scenarigo/scenarigo/issues/55))
- **template:** avoid to panic ([#54](https://github.com/scenarigo/scenarigo/issues/54))

### Features
- **cmd:** add list sub-command ([#51](https://github.com/scenarigo/scenarigo/issues/51))

## [v0.3.3] - 2020-06-17
### Bug Fixes
- **core:** add generated files to avoid the import error ([#41](https://github.com/scenarigo/scenarigo/issues/41))
- **deps:** update YAML library ( v1.7.12 => v1.7.15 ) ([#47](https://github.com/scenarigo/scenarigo/issues/47))
- **deps:** update YAML library ( v1.7.10 => v1.7.11 ) ([#42](https://github.com/scenarigo/scenarigo/issues/42))
- **deps:** update YAML library to fix a bug ( v1.7.9 => v1.7.10 ) ([#40](https://github.com/scenarigo/scenarigo/issues/40))
- **template:** fix processing for variadic arguments of function ([#48](https://github.com/scenarigo/scenarigo/issues/48))

## [v0.3.2] - 2020-06-15
### Bug Fixes
- **deps:** update YAML library to fix a bug ( v1.7.8 => v1.7.9 ) ([#39](https://github.com/scenarigo/scenarigo/issues/39))

## [v0.3.1] - 2020-06-12
### Bug Fixes
- **core:** fix ctx.Response() for http protocol ([#35](https://github.com/scenarigo/scenarigo/issues/35))
- **errors:** fix incorrect line number in YAML source ([#38](https://github.com/scenarigo/scenarigo/issues/38))

## [v0.3.0] - 2020-06-11
### Features
- **core:** support to output error with YAML ([#33](https://github.com/scenarigo/scenarigo/issues/33))

## [v0.2.0] - 2020-06-03
### Code Refactoring
- **core:** replace YAML libraries to goccy/go-yaml ([#31](https://github.com/scenarigo/scenarigo/issues/31))

### Features
- **core:** read YAML files only as scenarios ([#28](https://github.com/scenarigo/scenarigo/issues/28))
- **grpc:** enable to check header/trailer metadata of gRPC response ([#29](https://github.com/scenarigo/scenarigo/issues/29))
- **http:** enable to check HTTP response headers ([#30](https://github.com/scenarigo/scenarigo/issues/30))

### BREAKING CHANGE

change protocl.Protocol interface

## v0.1.0 - 2020-05-17
- first release


[v0.17.3]: https://github.com/scenarigo/scenarigo/compare/v0.17.2...v0.17.3
[v0.17.2]: https://github.com/scenarigo/scenarigo/compare/v0.17.1...v0.17.2
[v0.17.1]: https://github.com/scenarigo/scenarigo/compare/v0.17.0...v0.17.1
[v0.17.0]: https://github.com/scenarigo/scenarigo/compare/v0.16.2...v0.17.0
[v0.16.2]: https://github.com/scenarigo/scenarigo/compare/v0.16.1...v0.16.2
[v0.16.1]: https://github.com/scenarigo/scenarigo/compare/v0.16.0...v0.16.1
[v0.16.0]: https://github.com/scenarigo/scenarigo/compare/v0.15.1...v0.16.0
[v0.15.1]: https://github.com/scenarigo/scenarigo/compare/v0.15.0...v0.15.1
[v0.15.0]: https://github.com/scenarigo/scenarigo/compare/v0.14.2...v0.15.0
[v0.14.2]: https://github.com/scenarigo/scenarigo/compare/v0.14.1...v0.14.2
[v0.14.1]: https://github.com/scenarigo/scenarigo/compare/v0.14.0...v0.14.1
[v0.14.0]: https://github.com/scenarigo/scenarigo/compare/v0.13.2...v0.14.0
[v0.13.2]: https://github.com/scenarigo/scenarigo/compare/v0.13.1...v0.13.2
[v0.13.1]: https://github.com/scenarigo/scenarigo/compare/v0.13.0...v0.13.1
[v0.13.0]: https://github.com/scenarigo/scenarigo/compare/v0.12.8...v0.13.0
[v0.12.8]: https://github.com/scenarigo/scenarigo/compare/v0.12.7...v0.12.8
[v0.12.7]: https://github.com/scenarigo/scenarigo/compare/v0.12.6...v0.12.7
[v0.12.6]: https://github.com/scenarigo/scenarigo/compare/v0.12.5...v0.12.6
[v0.12.5]: https://github.com/scenarigo/scenarigo/compare/v0.12.4...v0.12.5
[v0.12.4]: https://github.com/scenarigo/scenarigo/compare/v0.12.3...v0.12.4
[v0.12.3]: https://github.com/scenarigo/scenarigo/compare/v0.12.2...v0.12.3
[v0.12.2]: https://github.com/scenarigo/scenarigo/compare/v0.12.1...v0.12.2
[v0.12.1]: https://github.com/scenarigo/scenarigo/compare/v0.12.0...v0.12.1
[v0.12.0]: https://github.com/scenarigo/scenarigo/compare/v0.11.2...v0.12.0
[v0.11.2]: https://github.com/scenarigo/scenarigo/compare/v0.11.1...v0.11.2
[v0.11.1]: https://github.com/scenarigo/scenarigo/compare/v0.11.0...v0.11.1
[v0.11.0]: https://github.com/scenarigo/scenarigo/compare/v0.10.0...v0.11.0
[v0.10.0]: https://github.com/scenarigo/scenarigo/compare/v0.9.0...v0.10.0
[v0.9.0]: https://github.com/scenarigo/scenarigo/compare/v0.8.1...v0.9.0
[v0.8.1]: https://github.com/scenarigo/scenarigo/compare/v0.8.0...v0.8.1
[v0.8.0]: https://github.com/scenarigo/scenarigo/compare/v0.7.0...v0.8.0
[v0.7.0]: https://github.com/scenarigo/scenarigo/compare/v0.6.3...v0.7.0
[v0.6.3]: https://github.com/scenarigo/scenarigo/compare/v0.6.2...v0.6.3
[v0.6.2]: https://github.com/scenarigo/scenarigo/compare/v0.6.1...v0.6.2
[v0.6.1]: https://github.com/scenarigo/scenarigo/compare/v0.6.0...v0.6.1
[v0.6.0]: https://github.com/scenarigo/scenarigo/compare/v0.5.1...v0.6.0
[v0.5.1]: https://github.com/scenarigo/scenarigo/compare/v0.5.0...v0.5.1
[v0.5.0]: https://github.com/scenarigo/scenarigo/compare/v0.4.0...v0.5.0
[v0.4.0]: https://github.com/scenarigo/scenarigo/compare/v0.3.3...v0.4.0
[v0.3.3]: https://github.com/scenarigo/scenarigo/compare/v0.3.2...v0.3.3
[v0.3.2]: https://github.com/scenarigo/scenarigo/compare/v0.3.1...v0.3.2
[v0.3.1]: https://github.com/scenarigo/scenarigo/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/scenarigo/scenarigo/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/scenarigo/scenarigo/compare/v0.1.0...v0.2.0
