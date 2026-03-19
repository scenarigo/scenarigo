package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/Yamashou/gqlgenc/clientv2"

	"github.com/scenarigo/scenarigo/scripts/cross-build/gen"
)

var minVer *semver.Version

func init() {
	var err error
	minVer, err = semver.NewVersion("1.25.0")
	if err != nil {
		panic(err)
	}
}

func main() {
	if err := printVersions(); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}

func printVersions() error {
	vers, err := getVers(os.Getenv("GITHUB_TOKEN"))
	if err != nil {
		return fmt.Errorf("failed to get Go versions: %w", err)
	}

	// Each major Go release is supported until there are two newer major releases.
	// https://go.dev/doc/devel/release#policy
	sort.Strings(vers)
	latest := semver.MustParse(vers[len(vers)-1])
	minSupportedVer := semver.MustParse(fmt.Sprintf("%d.%d.0", latest.Major(), latest.Minor()-1))
	for i, s := range vers {
		v := semver.MustParse(s)
		if !v.LessThan(minSupportedVer) {
			vers = vers[i:]
			break
		}
	}

	b, err := json.Marshal(vers)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}
	fmt.Println(string(b))
	return nil
}

func getVers(token string) ([]string, error) {
	github := &gen.Client{
		Client: clientv2.NewClient(http.DefaultClient, "https://api.github.com/graphql", func(ctx context.Context, req *http.Request, gqlInfo *clientv2.GQLRequestInfo, res interface{}, next clientv2.RequestInterceptorFunc) error {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			return next(ctx, req, gqlInfo, res)
		}),
	}

	ctx := context.Background()
	first := 50
	getTags, err := github.GetTags(ctx, "golang", "go", &first)
	if err != nil {
		if handledError, ok := err.(*clientv2.ErrorResponse); ok {
			return nil, fmt.Errorf("handled error: %sn", handledError.Error())
		}
		return nil, fmt.Errorf("unhandled error: %s", err.Error())
	}

	vers := make([]string, 0, first)
	for _, node := range getTags.Repository.Refs.Nodes {
		ver := strings.TrimPrefix(node.Name, "go")
		v, err := semver.NewVersion(ver)
		if err != nil {
			continue
		}
		if !v.LessThan(minVer) {
			vers = append(vers, v.Original())
		}
	}

	return vers, nil
}
