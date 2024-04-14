package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/Yamashou/gqlgenc/clientv2"

	"github.com/zoncoen/scenarigo/scripts/cross-build/gen"
)

var (
	token            = os.Getenv("GITHUB_TOKEN")
	ver              = os.Getenv("GO_VERSION")
	rootDir          = os.Getenv("PJ_ROOT")
	errImageNotFound = errors.New("image not found")
	go1_22_2         = semver.MustParse("1.22.2")
)

func main() {
	if err := release(); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}

func release() error {
	tag, err := imageTag(ver, token)
	if err != nil {
		if errors.Is(err, errImageNotFound) {
			fmt.Printf("golang-cross:v%s: %s\n", ver, err)
			return nil
		}
		return fmt.Errorf("failed to get image tag: %w", err)
	}

	v, err := semver.NewVersion(ver)
	if err != nil {
		return fmt.Errorf("failed to parse version: %w", err)
	}
	cc := "aarch64-apple-darwin23-clang"
	if v.LessThan(go1_22_2) {
		cc = "aarch64-apple-darwin22-clang"
	}

	if err := build(ver, tag, cc); err != nil {
		return fmt.Errorf("failed to build: %w", err)
	}
	return nil
}

func imageTag(ver, token string) (string, error) {
	github := &gen.Client{
		Client: clientv2.NewClient(http.DefaultClient, "https://api.github.com/graphql", func(ctx context.Context, req *http.Request, gqlInfo *clientv2.GQLRequestInfo, res interface{}, next clientv2.RequestInterceptorFunc) error {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			return next(ctx, req, gqlInfo, res)
		}),
	}

	ctx := context.Background()
	first := 100
	getTags, err := github.GetTags(ctx, "gythialy", "golang-cross", &first)
	if err != nil {
		if handledError, ok := err.(*clientv2.ErrorResponse); ok {
			return "", fmt.Errorf("handled error: %sn", handledError.Error())
		}
		return "", fmt.Errorf("unhandled error: %s", err.Error())
	}

	v := fmt.Sprintf("v%s", ver)
	prefix := fmt.Sprintf("%s-", v)
	var sv *semver.Version
	tags := []*semver.Version{}
	for _, node := range getTags.Repository.Refs.Nodes {
		if node.Name == v {
			sv = semver.MustParse(node.Name)
		}
		if strings.HasPrefix(node.Name, prefix) {
			tags = append(tags, semver.MustParse(node.Name))
		}
	}
	slices.SortFunc(tags, func(a, b *semver.Version) int {
		return -a.Compare(b)
	})
	if sv != nil {
		tags = append(tags, sv)
	}

	for _, tag := range tags {
		v := tag.Original()
		if err := exec.Command(
			"docker", "manifest", "inspect",
			fmt.Sprintf("ghcr.io/gythialy/golang-cross:%s", v),
		).Run(); err != nil {
			fmt.Println(v, "not found")
			continue
		}
		return v, nil
	}

	return "", errImageNotFound
}

//go:embed templates/goreleaser.yml.tmpl
var tmplBytes []byte

func build(ver, tag, cc string) error {
	if err := os.Mkdir(fmt.Sprintf("%s/assets", rootDir), 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpl := template.New("config").Delims("<<", ">>")
	tmpl, err := tmpl.Parse(string(tmplBytes))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	f, err := os.OpenFile(fmt.Sprintf("%s/.goreleaser.yml", rootDir), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return fmt.Errorf("failed to open .goreleaser.yml: %w", err)
	}
	defer f.Close()
	if err := tmpl.Execute(f, map[string]interface{}{
		"GoVersion": ver,
		"CC":        cc,
	}); err != nil {
		return fmt.Errorf("failed to create .goreleaser.yml: %w", err)
	}
	if err := goreleaser(ver, tag); err != nil {
		return fmt.Errorf("goreleaser failed (go%s, golang-cross:%s): %w", ver, tag, err)
	}

	return nil
}

func goreleaser(ver, tag string) error {
	args := []string{
		"run",
		"--rm", "--privileged",
		"-v", fmt.Sprintf("%s:/scenarigo", rootDir),
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"-w", "/scenarigo",
		"-e", fmt.Sprintf("SNAPSHOT=%s", os.Getenv("SNAPSHOT")),
		"--entrypoint", "bash",
	}

	// golang-cross has no matching manifest for linux/arm64/v8 in the manifest list entries.
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		args = append(args, "--platform", "linux/x86_64")
	}

	out, err := exec.Command(
		"docker",
		append(args,
			fmt.Sprintf("ghcr.io/gythialy/golang-cross:%s", tag),
			"/scenarigo/scripts/cross-build.sh",
		)...,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s:\n%s", err, out)
	}
	return nil
}
