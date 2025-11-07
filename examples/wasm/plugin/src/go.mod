module github.com/scenarigo/scenarigo/examples/wasm/plugin/src

go 1.24.0

toolchain go1.24.3

require (
	github.com/scenarigo/scenarigo v0.23.0
	google.golang.org/grpc v1.75.0
	google.golang.org/protobuf v1.36.8
)

require github.com/kr/text v0.2.0 // indirect

require (
	carvel.dev/ytt v0.50.0 // indirect
	dario.cat/mergo v1.0.2 // indirect
	github.com/bufbuild/protocompile v0.14.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/goccy/go-yaml v1.18.0 // indirect
	github.com/goccy/wasi-go v0.2.0 // indirect
	github.com/goccy/wasi-go-net v0.3.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/jhump/protoreflect v1.17.0 // indirect
	github.com/k14s/starlark-go v0.0.0-20200720175618-3a5c849cc368 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-encoding v0.0.2 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/stealthrocket/wazergo v0.19.1 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	github.com/tetratelabs/wazero v1.9.0 // indirect
	github.com/zoncoen/query-go v1.3.2 // indirect
	github.com/zoncoen/query-go/extractor/protobuf v0.1.4 // indirect
	github.com/zoncoen/query-go/extractor/yaml v0.2.2 // indirect
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
)

replace github.com/scenarigo/scenarigo v0.21.3 => ../../../..
