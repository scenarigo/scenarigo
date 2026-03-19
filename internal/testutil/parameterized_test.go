package testutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
)

func TestRunParameterizedTests_Success(t *testing.T) {
	var yml any
	values := []any{}
	executor := func(r Reporter, decode func(any)) func(Reporter, any) error {
		decode(&yml)
		return func(r Reporter, v any) error {
			values = append(values, v)
			if s, ok := v.(string); ok {
				if strings.HasPrefix(s, "ok-") {
					return nil
				}
			}
			return errors.New("ng")
		}
	}

	r := &reporter{}
	RunParameterizedTests(r, executor, "testdata/parameterized-success.yaml")
	if r.Failed() {
		t.Fatal("test failed")
	}

	if diff := cmp.Diff(
		yaml.MapSlice{
			{Key: "key", Value: "value"},
		},
		yml,
	); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(
		[]any{
			"ok-1", "ok-2", "ng-1", "ng-2",
		},
		values,
	); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}

func TestRunParameterizedTests_Failure(t *testing.T) {
	t.Run("file not found", func(t *testing.T) {
		r := &reporter{}
		executor := func(r Reporter, decode func(any)) func(Reporter, any) error {
			return func(r Reporter, v any) error {
				return nil
			}
		}
		RunParameterizedTests(r, executor, "testdata/not-found.yaml")
		if !r.Failed() {
			t.Fatal("test passed")
		}
	})

	t.Run("invalid YAML", func(t *testing.T) {
		r := &reporter{}
		executor := func(r Reporter, decode func(any)) func(Reporter, any) error {
			return func(r Reporter, v any) error {
				return nil
			}
		}
		RunParameterizedTests(r, executor, "testdata/parameterized-invalid.yaml")
		if !r.Failed() {
			t.Fatal("test passed")
		}
	})

	t.Run("failed parameter", func(t *testing.T) {
		values := []any{}
		executor := func(r Reporter, decode func(any)) func(Reporter, any) error {
			return func(r Reporter, v any) error {
				values = append(values, v)
				if s, ok := v.(string); ok {
					if s == "ok" {
						return nil
					}
				}
				return errors.New("ng")
			}
		}

		r := &reporter{}
		RunParameterizedTests(r, executor, "testdata/parameterized-failure.yaml")
		if !r.Failed() {
			t.Fatal("test passed")
		}

		if diff := cmp.Diff(
			[]any{
				"ng", "ok",
			},
			values,
		); diff != "" {
			t.Errorf("(-want +got):\n%s", diff)
		}
	})
}
