package assert

import (
	"context"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/scenarigo/scenarigo/errors"
)

func TestBuild(t *testing.T) {
	str := `
deps:
- name: scenarigo
  version:
    major: 1
    minor: 2
    patch: 3
  tags:
    - go
    - '{{$ == "test"}}'`
	var in interface{}
	if err := yaml.NewDecoder(strings.NewReader(str), yaml.UseOrderedMap()).Decode(&in); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	qs := []string{
		".deps[0].name",
		".deps[0].version.major",
		".deps[0].version.minor",
		".deps[0].version.patch",
		".deps[0].tags[0]",
		".deps[0].tags[1]",
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assertion, err := Build(ctx, in)
	if err != nil {
		t.Fatal(err)
	}

	type info struct {
		Deps []map[string]interface{} `yaml:"deps"`
	}

	t.Run("no assertion", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		assertion, err := Build(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		v := info{}
		if err := assertion.Assert(v); err != nil {
			t.Errorf("unexpected error: %s", err)
		}
	})
	t.Run("compare", func(t *testing.T) {
		if err := MustBuild(context.Background(), Greater(1)).Assert(2); err != nil {
			t.Fatal(err)
		}
		if err := MustBuild(context.Background(), GreaterOrEqual(1)).Assert(1); err != nil {
			t.Fatal(err)
		}
		if err := MustBuild(context.Background(), Less(2)).Assert(1); err != nil {
			t.Fatal(err)
		}
		if err := MustBuild(context.Background(), LessOrEqual(1)).Assert(1); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("ok", func(t *testing.T) {
		v := info{
			Deps: []map[string]interface{}{
				{
					"name": "scenarigo",
					"version": map[string]int{
						"major": 1,
						"minor": 2,
						"patch": 3,
					},
					"tags": []string{"go", "test"},
				},
			},
		}
		if err := assertion.Assert(v); err != nil {
			t.Errorf("unexpected error: %s", err)
		}
	})
	t.Run("ng", func(t *testing.T) {
		v := info{
			Deps: []map[string]interface{}{
				{
					"name": "Ruby on Rails",
					"version": map[string]int{
						"major": 2,
						"minor": 3,
						"patch": 4,
					},
					"tags": []string{"ruby", "http"},
				},
			},
		}
		err := assertion.Assert(v)
		if err == nil {
			t.Fatalf("expected error but no error")
		}
		var mperr *errors.MultiPathError
		if ok := errors.As(err, &mperr); !ok {
			t.Fatalf("expected errors.MultiPathError: %s", err)
		}
		if got, expect := len(mperr.Errs), len(qs); got != expect {
			t.Fatalf("expected %d but got %d", expect, got)
		}
		for i, e := range mperr.Errs {
			q := qs[i]
			if !strings.Contains(e.Error(), q) {
				t.Errorf(`"%s" does not contain "%s"`, e.Error(), q)
			}
		}
	})
	t.Run("assert nil", func(t *testing.T) {
		err := assertion.Assert(nil)
		if err == nil {
			t.Fatalf("expected error but no error")
		}
		var mperr *errors.MultiPathError
		if ok := errors.As(err, &mperr); !ok {
			t.Fatalf("expected errors.MultiPathError: %s", err)
		}
		if got, expect := len(mperr.Errs), len(qs); got != expect {
			t.Fatalf("expected %d but got %d", expect, got)
		}
		for i, e := range mperr.Errs {
			q := qs[i]
			if !strings.Contains(e.Error(), q) {
				t.Errorf(`"%s" does not contain "%s"`, e.Error(), q)
			}
		}
	})
	t.Run("options", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		assertion, err := Build(
			ctx, `{{aaa}}`,
			FromTemplate(map[string]string{"aaa": "foo"}),
			WithEqualers(EqualerFunc(func(a, b any) (bool, error) {
				return true, nil
			})),
		)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if err := assertion.Assert("bar"); err != nil {
			t.Errorf("unexpected error: %s", err)
		}
	})
	t.Run("use $", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		assertion, err := Build(ctx, `{{$ == "foo"}}`)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if err := assertion.Assert("foo"); err != nil {
			t.Errorf("unexpected error: %s", err)
		}
		// call Assert twice
		if err := assertion.Assert("bar"); err == nil {
			t.Error("no error")
		} else if got, expect := err.Error(), "assertion error"; got != expect {
			t.Errorf("expect %q but got %q", expect, got)
		}
	})
	t.Run("use $ twice", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		assertion, err := Build(ctx, `{{$ == $}}`)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if err := assertion.Assert("test"); err != nil {
			t.Errorf("unexpected error: %s", err)
		}
	})
	t.Run("assertion result is not boolean", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		assertion, err := Build(ctx, `{{$ + $}}`)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if err := assertion.Assert(1); err == nil {
			t.Error("no error")
		} else if got, expect := err.Error(), "assertion result must be a boolean value but got int64"; got != expect {
			t.Errorf("expect %q but got %q", expect, got)
		}
	})
	t.Run("left arrow function", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			assertion, err := Build(ctx, yaml.MapSlice{
				yaml.MapItem{
					Key: "{{call <-}}",
					Value: yaml.MapSlice{
						yaml.MapItem{
							Key:   "f",
							Value: `{{toUpper}}`,
						},
						yaml.MapItem{
							Key:   "arg",
							Value: `{{"foo"}}`,
						},
					},
				},
			},
				FromTemplate(map[string]any{
					"call":    &callFunc{},
					"toUpper": strings.ToUpper,
				}),
			)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if err := assertion.Assert("FOO"); err != nil {
				t.Errorf("unexpected error: %s", err)
			}
		})
		t.Run("failure", func(t *testing.T) {
			t.Run("build error: left arrow function not found", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				_, err := Build(ctx, yaml.MapSlice{
					yaml.MapItem{
						Key: "{{call <-}}",
						Value: yaml.MapSlice{
							yaml.MapItem{
								Key:   "f",
								Value: `{{toUpper}}`,
							},
							yaml.MapItem{
								Key:   "arg",
								Value: `{{"foo"}}`,
							},
						},
					},
				})
				if err == nil {
					t.Error("no error")
				} else if got, expect := err.Error(), `failed to build assertion: failed to execute: {{call <-}}: ".call" not found`; got != expect {
					t.Errorf("expect %q but got %q", expect, got)
				}
			})
			t.Run("build error: faild to unmarshal left arrow function arg", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				_, err := Build(ctx, yaml.MapSlice{
					yaml.MapItem{
						Key: "{{call <-}}",
						Value: yaml.MapSlice{
							yaml.MapItem{
								Key:   "func",
								Value: `{{toUpper}}`,
							},
							yaml.MapItem{
								Key:   "arg",
								Value: `{{"foo"}}`,
							},
						},
					},
				},
					FromTemplate(map[string]any{
						"call":    &callFunc{},
						"toUpper": strings.ToUpper,
					}),
				)
				if err == nil {
					t.Error("no error")
				} else if got, expect := err.Error(), `failed to build assertion: .'{{call <-}}': failed to execute left arrow function: failed to unmarshal argument: [1:1] unknown field "func"
>  1 | func: "{{func-0}}"
       ^
   2 | arg: foo`; got != expect {
					t.Errorf("expect %q but got %q", expect, got)
				}
			})
			t.Run("build error: invalid left arrow function arg", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				_, err := Build(ctx, yaml.MapSlice{
					yaml.MapItem{
						Key: "{{call <-}}",
						Value: yaml.MapSlice{
							yaml.MapItem{
								Key:   "f",
								Value: `{{toUpper}}`,
							},
							yaml.MapItem{
								Key:   "arg",
								Value: `{{"foo"}}`,
							},
						},
					},
				},
					FromTemplate(map[string]any{
						"call": &callFunc{},
					}),
				)
				if err == nil {
					t.Error("no error")
				} else if got, expect := err.Error(), `failed to build assertion: .'{{call <-}}'.'f': failed to execute left arrow function: failed to execute: {{toUpper}}: ".toUpper" not found`; got != expect {
					t.Errorf("expect %q but got %q", expect, got)
				}
			})
			t.Run("assertion error", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				assertion, err := Build(ctx, yaml.MapSlice{
					yaml.MapItem{
						Key: "{{call <-}}",
						Value: yaml.MapSlice{
							yaml.MapItem{
								Key:   "f",
								Value: `{{toUpper}}`,
							},
							yaml.MapItem{
								Key:   "arg",
								Value: `{{"foo"}}`,
							},
						},
					},
				},
					FromTemplate(map[string]any{
						"call":    &callFunc{},
						"toUpper": strings.ToUpper,
					}),
				)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if err := assertion.Assert("foo"); err == nil {
					t.Error("no error")
				} else if got, expect := err.Error(), `.'{{call <-}}': expected "FOO" but got "foo"`; got != expect {
					t.Errorf("expect %q but got %q", expect, got)
				}
			})
		})
	})
}

type callFunc struct{}

type callArg struct {
	F   interface{} `yaml:"f"`
	Arg string      `yaml:"arg"`
}

func (*callFunc) Exec(in interface{}) (interface{}, error) {
	arg, ok := in.(*callArg)
	if !ok {
		return nil, errors.New("arg must be a callArg")
	}
	f, ok := arg.F.(func(string) string)
	if !ok {
		return nil, errors.New("arg.f must be a func(string) string")
	}
	return f(arg.Arg), nil
}

func (*callFunc) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
	var arg callArg
	if err := unmarshal(&arg); err != nil {
		return nil, err
	}
	return &arg, nil
}
