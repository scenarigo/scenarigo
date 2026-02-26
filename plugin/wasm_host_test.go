package plugin

import (
	"bytes"
	gocontext "context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/reporter"
)

func buildWasmPlugin(t *testing.T) string {
	t.Helper()

	srcDir := filepath.Join("testdata", "wasm", "src")

	goModTidy := exec.Command("go", "mod", "tidy")
	goModTidy.Dir = srcDir
	if out, err := goModTidy.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", string(out), err)
	}

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

	return filepath.Join(srcDir, "main.wasm")
}

func TestWasmHost(t *testing.T) {
	wasmPath := buildWasmPlugin(t)
	plg, err := openWasmPlugin(wasmPath)
	if err != nil {
		t.Fatal(err)
	}
	wasmPlugin, ok := plg.(*WasmPlugin)
	if !ok {
		t.Fatalf("failed to get wasm plugin: %T", plg)
	}
	defer wasmPlugin.Close()
	r := reporter.FromT(t)
	ctx := context.New(r)
	setup := wasmPlugin.GetSetup()
	var teardown func(*context.Context)
	if setup != nil {
		ctx, teardown = setup(ctx)
	}
	if teardown != nil {
		defer teardown(ctx)
	}

	setupEachScenario := wasmPlugin.GetSetupEachScenario()
	var scenarioTeardown func(*context.Context)
	if setupEachScenario != nil {
		ctx, scenarioTeardown = setupEachScenario(ctx)
	}
	if scenarioTeardown != nil {
		defer scenarioTeardown(ctx)
	}

	t.Run("value", func(t *testing.T) {
		t.Run("int", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Int")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != int(1) {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("int8", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Int8")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != int8(2) {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("int16", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Int16")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != int16(3) {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("int32", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Int32")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != int32(4) {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("int64", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Int64")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != int64(5) {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("uint", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Uint")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != uint(6) {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("uint8", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Uint8")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != uint8(7) {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("uint16", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Uint16")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != uint16(8) {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("uint32", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Uint32")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != uint32(9) {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("uint64", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Uint64")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != uint64(10) {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("float32", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Float32")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != float32(11) {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("float64", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Float64")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != float64(12) {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("uintptr", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Uintptr")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != uintptr(13) {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("bool", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Bool")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != true {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("string", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("String")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != "hello" {
				t.Fatalf("failed to get value: %v", v)
			}
		})
		t.Run("[]byte", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Bytes")
			if !ok {
				t.Fatalf("failed to get value")
			}
			b, ok := v.([]byte)
			if !ok {
				t.Fatalf("failed to get bytes value: %T", v)
			}
			if !bytes.Equal(b, []byte("world")) {
				t.Fatalf("failed to get value: %q", b)
			}
		})
		t.Run("map", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Map")
			if !ok {
				t.Fatalf("failed to get value")
			}
			m, ok := v.(map[string]any)
			if !ok {
				t.Fatalf("failed to get bytes value: %T", v)
			}
			if !reflect.DeepEqual(m, map[string]any{
				"a": "x",
				"b": float64(1),
			}) {
				t.Fatalf("failed to get value: %+v", m)
			}
		})
		t.Run("slice", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Slice")
			if !ok {
				t.Fatalf("failed to get value")
			}
			slice, ok := v.([]any)
			if !ok {
				t.Fatalf("failed to get bytes value: %T", v)
			}
			expected := []any{
				float64(1),
				float64(-2),
				float64(3.14),
				true,
				"hello",
			}
			if !reflect.DeepEqual(slice, expected) {
				t.Fatalf("failed to get value: %+v", slice)
			}
		})
		t.Run("array", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Array")
			if !ok {
				t.Fatalf("failed to get value")
			}
			arr, ok := v.([2]int64)
			if !ok {
				t.Fatalf("failed to get bytes value: %T", v)
			}
			if !reflect.DeepEqual(arr, [2]int64{1, 2}) {
				t.Fatalf("failed to get value: %+v", arr)
			}
		})
		t.Run("struct", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Struct")
			if !ok {
				t.Fatalf("failed to get value")
			}
			st, ok := v.(*StructValue)
			if !ok {
				t.Fatalf("failed to get bytes value: %T", v)
			}
			x, ok := st.ExtractByKey("X")
			if !ok {
				t.Fatalf("failed to get x field value")
			}
			if x != 1 {
				t.Fatalf("failed to get x: %+v", x)
			}
			y, ok := st.ExtractByKey("Y")
			if !ok {
				t.Fatalf("failed to get y field value")
			}
			if y != "hello" {
				t.Fatalf("failed to get x: %+v", y)
			}
		})
		t.Run("structptr", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("StructPtr")
			if !ok {
				t.Fatalf("failed to get value")
			}
			st, ok := v.(*StructValue)
			if !ok {
				t.Fatalf("failed to get bytes value: %T", v)
			}
			x, ok := st.ExtractByKey("X")
			if !ok {
				t.Fatalf("failed to get x field value")
			}
			if x != 1 {
				t.Fatalf("failed to get x: %+v", x)
			}
			y, ok := st.ExtractByKey("Y")
			if !ok {
				t.Fatalf("failed to get y field value")
			}
			if y != "hello" {
				t.Fatalf("failed to get x: %+v", y)
			}
		})
		t.Run("structchain", func(t *testing.T) {
			chain, ok := wasmPlugin.ExtractByKey("StructChain")
			if !ok {
				t.Fatalf("failed to get value")
			}
			stchain, ok := chain.(*StructValue)
			if !ok {
				t.Fatalf("failed to get bytes value: %T", chain)
			}
			x, ok := stchain.ExtractByKey("X")
			if !ok {
				t.Fatalf("failed to get x field value")
			}
			xst, ok := x.(*StructValue)
			if !ok {
				t.Fatalf("failed to get bytes value: %T", x)
			}
			y, ok := xst.ExtractByKey("Y")
			if !ok {
				t.Fatalf("failed to get x field value")
			}
			yst, ok := y.(*StructValue)
			if !ok {
				t.Fatalf("failed to get bytes value: %T", y)
			}
			z, ok := yst.ExtractByKey("Z")
			if !ok {
				t.Fatalf("failed to get x field value")
			}
			if z != 10 {
				t.Fatalf("failed to get z: %+v", z)
			}
		})
		t.Run("any", func(t *testing.T) {
			v, ok := wasmPlugin.ExtractByKey("Any")
			if !ok {
				t.Fatalf("failed to get value")
			}
			if v != int(1) {
				t.Fatalf("failed to get value: %v(%T)", v, v)
			}
		})
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
		if barValue != int(2) {
			t.Fatalf("failed to get value from Bar: %v", barValue)
		}
	})
	t.Run("grpc", func(t *testing.T) {
		client, ok := wasmPlugin.ExtractByKey("EchoClient")
		if !ok {
			t.Fatalf("failed to get EchoClient")
		}
		wasmValue, ok := client.(*StructValue)
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
		res, st, err := wasmValue.Invoke(gocontext.Background(), "Echo", protoMsg)
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

func TestWasmPluginClose(t *testing.T) {
	wasmPath := buildWasmPlugin(t)

	t.Run("close completes without deadlock", func(t *testing.T) {
		// This test verifies that Close() does not deadlock when the WASM module
		// is idle (waiting for the next request in the read_length host function).
		// Before the fix, cancelFn() could not interrupt the Go channel receive
		// (<-reqCh) in read_length, causing Close() to hang forever.
		plg, err := openWasmPlugin(wasmPath)
		if err != nil {
			t.Fatal(err)
		}

		done := make(chan struct{})
		go func() {
			plg.Close()
			close(done)
		}()

		select {
		case <-done:
			// Close() completed successfully.
		case <-time.After(10 * time.Second):
			t.Fatal("Close() did not complete within 10 seconds; likely deadlocked")
		}
	})

	t.Run("close is idempotent", func(t *testing.T) {
		plg, err := openWasmPlugin(wasmPath)
		if err != nil {
			t.Fatal(err)
		}

		// Calling Close() multiple times must not panic or deadlock.
		plg.Close()
		plg.Close()
	})

	t.Run("write returns error after close", func(t *testing.T) {
		plg, err := openWasmPlugin(wasmPath)
		if err != nil {
			t.Fatal(err)
		}
		wasmPlg := plg.(*WasmPlugin)
		wasmPlg.Close()

		if err := wasmPlg.write([]byte("test")); err == nil {
			t.Fatal("write() should return an error after Close()")
		}
	})

	t.Run("read returns error after close", func(t *testing.T) {
		plg, err := openWasmPlugin(wasmPath)
		if err != nil {
			t.Fatal(err)
		}
		wasmPlg := plg.(*WasmPlugin)
		wasmPlg.Close()

		if _, err := wasmPlg.read(); err == nil {
			t.Fatal("read() should return an error after Close()")
		}
	})
}
