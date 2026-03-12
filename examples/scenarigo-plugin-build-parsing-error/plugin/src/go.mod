module github.com/ShotaroTakami/scenarigo/examples/scenarigo-plugin-build-parsing-error/plugin

go 1.25.0

toolchain go1.25.6

require github.com/scenarigo/scenarigo v0.24.1-0.20251205075024-479115e02f7a

require (
	carvel.dev/ytt v0.50.0 // indirect
	dario.cat/mergo v1.0.2 // indirect
	github.com/bufbuild/protocompile v0.14.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/goccy/go-yaml v1.19.0 // indirect
	github.com/goccy/wasi-go v0.3.2 // indirect
	github.com/goccy/wasi-go-net v0.3.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-version v1.8.0 // indirect
	github.com/jhump/protoreflect v1.17.0 // indirect
	github.com/jhump/protoreflect/v2 v2.0.0-beta.1 // indirect
	github.com/k14s/starlark-go v0.0.0-20200720175618-3a5c849cc368 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-encoding v0.0.2 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/stealthrocket/wazergo v0.19.1 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/tetratelabs/wazero v1.10.1 // indirect
	github.com/zoncoen/query-go v1.3.2 // indirect
	github.com/zoncoen/query-go/extractor/protobuf v0.1.4 // indirect
	github.com/zoncoen/query-go/extractor/yaml v0.2.2 // indirect
	go.opentelemetry.io/otel v1.39.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.39.0 // indirect
	golang.org/x/mod v0.30.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/grpc v1.77.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	github.com/google/go-cmp v0.7.0
	./genproto-local
	github.com/sergi/go-diff v1.4.0
	github.com/sosedoff/gitkit v0.4.0
	github.com/spf13/cobra v1.10.2
	github.com/golang/mock v1.7.0-rc.1
	github.com/inconshreveable/mousetrap v1.1.0
	github.com/spf13/pflag v1.0.10
	github.com/gofrs/uuid v4.4.0+incompatible
	golang.org/x/crypto v0.45.0
)

replace carvel.dev/ytt v0.52.1 => carvel.dev/ytt v0.50.0

replace google.golang.org/genproto/googleapis/rpc v0.0.0-20260203192932-546029d2fa20 => google.golang.org/genproto/googleapis/rpc v0.0.0-20251029180050-ab9386a59fda

replace google.golang.org/genproto v0.0.0-20251202230838-ff82c1b0f217 => ./genproto-local
