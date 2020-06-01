package maputil

import (
	"reflect"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestConvertStringsMapSlice(t *testing.T) {
	mapSlice, err := ConvertStringsMapSlice(yaml.MapSlice{
		{
			Key:   "a",
			Value: 1,
		},
		{
			Key:   "b",
			Value: 2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mapSlice) != 2 {
		t.Fatalf("failed to convert yaml.MapSlice. expected length is 2 but got %d", len(mapSlice))
	}
	if mapSlice[0].Key != "a" {
		t.Fatalf("unexpected key %s", mapSlice[0].Key)
	}
	if !reflect.DeepEqual(mapSlice[0].Value, []string{"1"}) {
		t.Fatalf("unexpected value %s", mapSlice[0].Value)
	}
	if mapSlice[1].Key != "b" {
		t.Fatalf("unexpected key %s", mapSlice[1].Key)
	}
	if !reflect.DeepEqual(mapSlice[1].Value, []string{"2"}) {
		t.Fatalf("unexpected value %s", mapSlice[1].Value)
	}
}
