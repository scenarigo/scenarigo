package schema

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sergi/go-diff/diffmatchpatch"
)

func TestOrderedMap_Set(t *testing.T) {
	tests := map[string]struct {
		m      OrderedMap[string, int]
		key    string
		value  int
		expect OrderedMap[string, int]
	}{
		"new key": {
			m: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 0,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "foo",
						Value: 1,
					},
				},
			},
			key:   "bar",
			value: 2,
			expect: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 0,
					"bar": 1,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "foo",
						Value: 1,
					},
					{
						Key:   "bar",
						Value: 2,
					},
				},
			},
		},
		"exists key": {
			m: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 0,
					"bar": 1,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "foo",
						Value: 1,
					},
					{
						Key:   "bar",
						Value: 2,
					},
				},
			},
			key:   "foo",
			value: 111,
			expect: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 0,
					"bar": 1,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "foo",
						Value: 111,
					},
					{
						Key:   "bar",
						Value: 2,
					},
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			test.m.Set(test.key, test.value)
			if diff := cmp.Diff(test.expect, test.m, cmp.AllowUnexported(OrderedMap[string, int]{})); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOrderedMap_Get(t *testing.T) {
	tests := map[string]struct {
		m      OrderedMap[string, int]
		key    string
		expect int
		ok     bool
	}{
		"success": {
			m: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 0,
					"bar": 1,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "foo",
						Value: 1,
					},
					{
						Key:   "bar",
						Value: 2,
					},
				},
			},
			key:    "foo",
			expect: 1,
			ok:     true,
		},
		"not found": {
			m:   OrderedMap[string, int]{},
			key: "foo",
		},
		"invalid idx": {
			m: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 0,
				},
			},
			key: "foo",
		},
		"invalid key": {
			m: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 1,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "foo",
						Value: 1,
					},
					{
						Key:   "bar",
						Value: 2,
					},
				},
			},
			key: "foo",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := test.m.Get(test.key)
			if expect := test.expect; got != expect {
				t.Errorf("expect %d but got %d", expect, got)
			}
			if got, expect := ok, test.ok; got != expect {
				t.Errorf("expect %t but got %t", expect, got)
			}
		})
	}
}

func TestOrderedMap_Delete(t *testing.T) {
	tests := map[string]struct {
		m      OrderedMap[string, int]
		key    string
		expect OrderedMap[string, int]
		ok     bool
	}{
		"not found": {
			m: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 0,
					"bar": 1,
					"baz": 2,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "foo",
						Value: 1,
					},
					{
						Key:   "bar",
						Value: 2,
					},
					{
						Key:   "baz",
						Value: 3,
					},
				},
			},
			key: "hoge",
			expect: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 0,
					"bar": 1,
					"baz": 2,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "foo",
						Value: 1,
					},
					{
						Key:   "bar",
						Value: 2,
					},
					{
						Key:   "baz",
						Value: 3,
					},
				},
			},
			ok: false,
		},
		"delete": {
			m: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 0,
					"bar": 1,
					"baz": 2,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "foo",
						Value: 1,
					},
					{
						Key:   "bar",
						Value: 2,
					},
					{
						Key:   "baz",
						Value: 3,
					},
				},
			},
			key: "bar",
			expect: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 0,
					"baz": 1,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "foo",
						Value: 1,
					},
					{
						Key:   "baz",
						Value: 3,
					},
				},
			},
			ok: true,
		},
		"delete head": {
			m: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 0,
					"bar": 1,
					"baz": 2,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "foo",
						Value: 1,
					},
					{
						Key:   "bar",
						Value: 2,
					},
					{
						Key:   "baz",
						Value: 3,
					},
				},
			},
			key: "foo",
			expect: OrderedMap[string, int]{
				idx: map[string]int{
					"bar": 0,
					"baz": 1,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "bar",
						Value: 2,
					},
					{
						Key:   "baz",
						Value: 3,
					},
				},
			},
			ok: true,
		},
		"delete tail": {
			m: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 0,
					"bar": 1,
					"baz": 2,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "foo",
						Value: 1,
					},
					{
						Key:   "bar",
						Value: 2,
					},
					{
						Key:   "baz",
						Value: 3,
					},
				},
			},
			key: "baz",
			expect: OrderedMap[string, int]{
				idx: map[string]int{
					"foo": 0,
					"bar": 1,
				},
				items: []OrderedMapItem[string, int]{
					{
						Key:   "foo",
						Value: 1,
					},
					{
						Key:   "bar",
						Value: 2,
					},
				},
			},
			ok: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ok := test.m.Delete(test.key)
			if got, expect := ok, test.ok; got != expect {
				t.Errorf("expect %t but got %t", expect, got)
			}
			if diff := cmp.Diff(test.expect, test.m, cmp.AllowUnexported(OrderedMap[string, int]{})); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOrderedMap_Len(t *testing.T) {
	m := NewOrderedMap[string, int]()
	m.Set("foo", 1)
	m.Set("bar", 2)
	m.Delete("foo")
	if got, expect := m.Len(), 1; got != expect {
		t.Errorf("expect %d but got %d", expect, got)
	}
}

func TestOrderedMap_ToMap(t *testing.T) {
	m := OrderedMap[string, int]{
		idx: map[string]int{
			"foo": 0,
			"bar": 1,
		},
		items: []OrderedMapItem[string, int]{
			{
				Key:   "foo",
				Value: 1,
			},
			{
				Key:   "bar",
				Value: 2,
			},
		},
	}
	got := m.ToMap()
	expect := map[string]int{
		"foo": 1,
		"bar": 2,
	}
	if diff := cmp.Diff(expect, got); diff != "" {
		t.Errorf("differs (-want +got):\n%s", diff)
	}
}

func TestOrderedMap_ToSlice(t *testing.T) {
	m := OrderedMap[string, int]{
		idx: map[string]int{
			"foo": 0,
			"bar": 1,
		},
		items: []OrderedMapItem[string, int]{
			{
				Key:   "foo",
				Value: 1,
			},
			{
				Key:   "bar",
				Value: 2,
			},
		},
	}
	got := m.ToSlice()
	expect := m.items
	if diff := cmp.Diff(expect, got); diff != "" {
		t.Errorf("differs (-want +got):\n%s", diff)
	}
}

type testStruct struct {
	N string `yaml:"name,omitempty"`
}

func TestOrderedMap_UnmarshalYAML(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			in     string
			expect OrderedMap[string, testStruct]
		}{
			"empty": {
				expect: OrderedMap[string, testStruct]{},
			},
			"success": {
				in: `
1:
  name: foo
false: {}
`,
				expect: OrderedMap[string, testStruct]{
					idx: map[string]int{
						"1":     0,
						"false": 1,
					},
					items: []OrderedMapItem[string, testStruct]{
						{
							Key: "1",
							Value: testStruct{
								N: "foo",
							},
						},
						{
							Key: "false",
						},
					},
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				var got OrderedMap[string, testStruct]
				if err := got.UnmarshalYAML([]byte(test.in)); err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if diff := cmp.Diff(test.expect, got, cmp.AllowUnexported(OrderedMap[string, testStruct]{})); diff != "" {
					t.Errorf("differs (-want +got):\n%s", diff)
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			in     string
			expect string
		}{
			"unknow field": {
				in: `
1:
  aaa: foo
`,
				expect: `[1:1] unknown field "aaa"
>  1 | aaa: foo
       ^
`,
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				var m OrderedMap[string, testStruct]
				err := m.UnmarshalYAML([]byte(test.in))
				if err == nil {
					t.Fatal("no error")
				} else {
					if got, expect := err.Error(), test.expect; got != expect {
						dmp := diffmatchpatch.New()
						diffs := dmp.DiffMain(expect, got, false)
						t.Errorf("error differs:\n%s", dmp.DiffPrettyText(diffs))
					}
				}
			})
		}
	})
}

func TestOrderedMap_MarshalYAML(t *testing.T) {
	tests := map[string]struct {
		in     OrderedMap[string, testStruct]
		expect string
	}{
		"success": {
			in: OrderedMap[string, testStruct]{
				items: []OrderedMapItem[string, testStruct]{
					{
						Key: "foo",
						Value: testStruct{
							N: "FOO",
						},
					},
					{
						Key: "bar",
						Value: testStruct{
							N: "BAR",
						},
					},
				},
			},
			expect: `foo:
  name: FOO
bar:
  name: BAR
`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			b, err := test.in.MarshalYAML()
			if err != nil {
				t.Fatal(err)
			}
			if got, expect := string(b), test.expect; got != expect {
				dmp := diffmatchpatch.New()
				diffs := dmp.DiffMain(expect, got, false)
				t.Errorf("differs:\n%s", dmp.DiffPrettyText(diffs))
			}
		})
	}
}

func TestOrderedMap_IsZero(t *testing.T) {
	m := NewOrderedMap[string, int]()
	if got, expect := m.IsZero(), true; got != expect {
		t.Errorf("expect %t but got %t", expect, got)
	}
	m.Set("foo", 1)
	if got, expect := m.IsZero(), false; got != expect {
		t.Errorf("expect %t but got %t", expect, got)
	}
}
