package plugin

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/reporter"
)

func TestWasmHost(t *testing.T) {
	srcDir := filepath.Join("testdata", "wasm", "src")
	cmd := exec.Command("go", "build", "-o", "main.wasm", ".")
	cmd.Env = append(os.Environ(), []string{
		"GOOS=wasip1",
		"GOARCH=wasm",
	}...)
	cmd.Dir = srcDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v", string(out), err)
	}
	plg, err := openWasmPlugin(filepath.Join(srcDir, "main.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	wasmPlugin, ok := plg.(*WasmPlugin)
	if !ok {
		t.Fatalf("failed to get wasm plugin: %T", plg)
	}
	r := reporter.FromT(t)
	ctx := context.New(r)
	ctx, teardown := wasmPlugin.GetSetup()(ctx)
	defer teardown(ctx)

	ctx, scenarioTeardown := wasmPlugin.GetSetupEachScenario()(ctx)
	defer scenarioTeardown(ctx)

	t.Run("value", func(t *testing.T) {
		fooValue, ok := wasmPlugin.ExtractByKey("Foo")
		if !ok {
			t.Fatalf("failed to get Foo value")
		}
		if fooValue != 1 {
			t.Fatalf("failed to get Foo value: %v", fooValue)
		}
	})
	t.Run("func", func(t *testing.T) {
		bar, ok := wasmPlugin.ExtractByKey("Bar")
		if !ok {
			t.Fatalf("failed to get Bar value")
		}
		ret := reflect.ValueOf(bar).Call([]reflect.Value{})
		if len(ret) != 1 {
			t.Fatalf("failed to get return value from Bar")
		}
		barValue := ret[0].Interface()
		if barValue != 2 {
			t.Fatalf("failed to get value from Bar: %v", barValue)
		}
	})
	t.Run("grpc", func(t *testing.T) {
		client, ok := wasmPlugin.ExtractByKey("EchoClient")
		if !ok {
			t.Fatalf("failed to get EchoClient")
		}
		wasmValue, ok := client.(*WasmValue)
		if !ok {
			t.Fatalf("failed to get WasmValue: %T", client)
		}
		if !wasmValue.ExistsMethod("Echo") {
			t.Fatal("failed to get exists method")
		}
		type Body struct {
			MessageID   string `json:"messageId"`
			MessageBody string `json:"messageBody"`
		}
		requestBody := &Body{
			MessageID:   "hello",
			MessageBody: "world",
		}
		msg, err := json.Marshal(requestBody)
		if err != nil {
			t.Fatal(err)
		}
		protoMsg, err := wasmValue.BuildRequestMessage("Echo", msg)
		if err != nil {
			t.Fatal(err)
		}
		res, st, err := wasmValue.Invoke("Echo", protoMsg)
		if err != nil {
			t.Fatal(err)
		}
		if st.Code() != codes.OK {
			t.Fatalf("failed to get status code: %s", st.Code())
		}
		encoded, err := protojson.Marshal(res)
		if err != nil {
			t.Fatal(err)
		}
		var responseBody Body
		if err := json.Unmarshal(encoded, &responseBody); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(requestBody, &responseBody) {
			t.Fatalf("failed to get encho response: %q", encoded)
		}
	})
}
