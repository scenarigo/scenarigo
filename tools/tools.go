// +build tools

package tools

import (
	_ "github.com/Songmu/gocredits/cmd/gocredits"
	_ "github.com/git-chglog/git-chglog/cmd/git-chglog"
	_ "github.com/golang/mock/mockgen"
	_ "github.com/golang/protobuf/protoc-gen-go"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/google/go-licenses"
	_ "github.com/x-motemen/gobump/cmd/gobump"
	_ "github.com/zoncoen/goprotoyamltag"
	_ "github.com/zoncoen/gotypenames"
)
