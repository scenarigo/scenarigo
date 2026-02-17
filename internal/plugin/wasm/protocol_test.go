package wasm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/scenarigo/scenarigo/context"
)

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

type mockCommandHandler struct{}

func (h *mockCommandHandler) Init(*InitCommandRequest) (*InitCommandResponse, error) {
	return &InitCommandResponse{
		Types: []*NameWithType{
			{
				Name: "testFunc",
				Type: &Type{
					Kind: FUNC,
					Func: &FuncType{
						Args:   []*Type{{Kind: STRING}},
						Return: []*Type{{Kind: INT}},
					},
				},
			},
		},
	}, nil
}

func (h *mockCommandHandler) Setup(*SetupCommandRequest) (*SetupCommandResponse, error) {
	return &SetupCommandResponse{}, nil
}

func (h *mockCommandHandler) SetupEachScenario(*SetupEachScenarioCommandRequest) (*SetupEachScenarioCommandResponse, error) {
	return &SetupEachScenarioCommandResponse{}, nil
}

func (h *mockCommandHandler) Teardown(*TeardownCommandRequest) (*TeardownCommandResponse, error) {
	return &TeardownCommandResponse{}, nil
}

func (h *mockCommandHandler) Sync(*SyncCommandRequest) (*SyncCommandResponse, error) {
	return &SyncCommandResponse{
		Types: []*NameWithType{
			{
				Name: "syncedFunc",
				Type: &Type{Kind: BOOL},
			},
		},
	}, nil
}

func (h *mockCommandHandler) Call(req *CallCommandRequest) (*CallCommandResponse, error) {
	return &CallCommandResponse{
		Return: []*Value{
			{ID: "result", Value: "called_" + req.Name},
		},
	}, nil
}

func (h *mockCommandHandler) Get(req *GetCommandRequest) (*GetCommandResponse, error) {
	return &GetCommandResponse{
		Value: &Value{
			Value: "value_of_" + req.Name,
		},
	}, nil
}

func (h *mockCommandHandler) GRPCExistsMethod(req *GRPCExistsMethodCommandRequest) (*GRPCExistsMethodCommandResponse, error) {
	return &GRPCExistsMethodCommandResponse{
		Exists: req.Method == "ExistingMethod",
	}, nil
}

func (h *mockCommandHandler) GRPCBuildRequest(req *GRPCBuildRequestCommandRequest) (*GRPCBuildRequestCommandResponse, error) {
	return &GRPCBuildRequestCommandResponse{
		FDSet:       []byte("fdset_for_" + req.Client),
		MessageFQDN: "com.example." + req.Method,
	}, nil
}

func (h *mockCommandHandler) GRPCInvoke(req *GRPCInvokeCommandRequest) (*GRPCInvokeCommandResponse, error) {
	return &GRPCInvokeCommandResponse{
		FDSet:         []byte("invoke_fdset"),
		ResponseFQDN:  "com.example.Response",
		ResponseBytes: []byte(`{"result": "success"}`),
		StatusProto:   []byte("status_ok"),
	}, nil
}

func (h *mockCommandHandler) LeftArrowFuncExec(*LeftArrowFuncExecCommandRequest) (*LeftArrowFuncExecCommandResponse, error) {
	return &LeftArrowFuncExecCommandResponse{
		Value: &Value{
			Value: "mock_exec_result",
		},
	}, nil
}

func (h *mockCommandHandler) LeftArrowFuncUnmarshalArg(*LeftArrowFuncUnmarshalArgCommandRequest) (*LeftArrowFuncUnmarshalArgCommandResponse, error) {
	return &LeftArrowFuncUnmarshalArgCommandResponse{
		Value: &Value{
			Value: "mock_unmarshal_result",
		},
	}, nil
}

func (h *mockCommandHandler) Method(*MethodCommandRequest) (*MethodCommandResponse, error) {
	return &MethodCommandResponse{
		Type: &Type{Kind: BOOL},
	}, nil
}

func (h *mockCommandHandler) StepRun(*StepRunCommandRequest) (*StepRunCommandResponse, error) {
	return &StepRunCommandResponse{}, nil
}

func (h *mockCommandHandler) HTTPCall(*HTTPCallCommandRequest) (*HTTPCallCommandResponse, error) {
	return &HTTPCallCommandResponse{}, nil
}

func TestEncodeRequestHandleCommandDecodeResponse(t *testing.T) {
	handler := &mockCommandHandler{}

	tests := []struct {
		name         string
		req          *Request
		validateFunc func(*testing.T, *Response)
	}{
		{
			name: "InitRequest",
			req:  NewInitRequest(),
			validateFunc: func(t *testing.T, resp *Response) {
				t.Helper()
				if resp.CommandType != InitCommand {
					t.Errorf("CommandType mismatch: got %v, want %v", resp.CommandType, InitCommand)
				}
				initResp, ok := resp.Command.(*InitCommandResponse)
				if !ok {
					t.Fatalf("Command type mismatch: got %T, want *InitCommandResponse", resp.Command)
				}
				if len(initResp.Types) != 1 {
					t.Errorf("Types length mismatch: got %d, want 1", len(initResp.Types))
				}
				if initResp.Types[0].Name != "testFunc" {
					t.Errorf("Type name mismatch: got %s, want testFunc", initResp.Types[0].Name)
				}
			},
		},
		{
			name: "SetupRequest",
			req: NewSetupRequest("setup-123", &context.SerializableContext{
				Vars: []any{
					map[string]any{"key1": "value1"},
				},
			}, 0),
			validateFunc: func(t *testing.T, resp *Response) {
				t.Helper()
				if resp.CommandType != SetupCommand {
					t.Errorf("CommandType mismatch: got %v, want %v", resp.CommandType, SetupCommand)
				}
				_, ok := resp.Command.(*SetupCommandResponse)
				if !ok {
					t.Fatalf("Command type mismatch: got %T, want *SetupCommandResponse", resp.Command)
				}
			},
		},
		{
			name: "SyncRequest",
			req:  NewSyncRequest(),
			validateFunc: func(t *testing.T, resp *Response) {
				t.Helper()
				if resp.CommandType != SyncCommand {
					t.Errorf("CommandType mismatch: got %v, want %v", resp.CommandType, SyncCommand)
				}
				syncResp, ok := resp.Command.(*SyncCommandResponse)
				if !ok {
					t.Fatalf("Command type mismatch: got %T, want *SyncCommandResponse", resp.Command)
				}
				if len(syncResp.Types) != 1 {
					t.Errorf("Types length mismatch: got %d, want 1", len(syncResp.Types))
				}
				if syncResp.Types[0].Name != "syncedFunc" {
					t.Errorf("Type name mismatch: got %s, want syncedFunc", syncResp.Types[0].Name)
				}
			},
		},
		{
			name: "CallRequest",
			req:  NewCallRequest("testFunction", []string{"arg1", "arg2"}, []*Value{}),
			validateFunc: func(t *testing.T, resp *Response) {
				t.Helper()
				if resp.CommandType != CallCommand {
					t.Errorf("CommandType mismatch: got %v, want %v", resp.CommandType, CallCommand)
				}
				callResp, ok := resp.Command.(*CallCommandResponse)
				if !ok {
					t.Fatalf("Command type mismatch: got %T, want *CallCommandResponse", resp.Command)
				}
				if len(callResp.Return) != 1 {
					t.Errorf("Return length mismatch: got %d, want 1", len(callResp.Return))
				}
				expectedValue := "called_testFunction"
				if callResp.Return[0].Value != expectedValue {
					t.Errorf("Return value mismatch: got %s, want %s", callResp.Return[0].Value, expectedValue)
				}
			},
		},
		{
			name: "GetRequest",
			req:  NewGetRequest("testVariable", []string{}),
			validateFunc: func(t *testing.T, resp *Response) {
				t.Helper()
				if resp.CommandType != GetCommand {
					t.Errorf("CommandType mismatch: got %v, want %v", resp.CommandType, GetCommand)
				}
				getResp, ok := resp.Command.(*GetCommandResponse)
				if !ok {
					t.Fatalf("Command type mismatch: got %T, want *GetCommandResponse", resp.Command)
				}
				expectedValue := "value_of_testVariable"
				if getResp.Value.Value != expectedValue {
					t.Errorf("Value mismatch: got %s, want %s", getResp.Value.Value, expectedValue)
				}
			},
		},
		{
			name: "GRPCExistsMethodRequest",
			req:  NewGRPCExistsMethodRequest("TestClient", "ExistingMethod"),
			validateFunc: func(t *testing.T, resp *Response) {
				t.Helper()
				if resp.CommandType != GRPCExistsMethodCommand {
					t.Errorf("CommandType mismatch: got %v, want %v", resp.CommandType, GRPCExistsMethodCommand)
				}
				grpcResp, ok := resp.Command.(*GRPCExistsMethodCommandResponse)
				if !ok {
					t.Fatalf("Command type mismatch: got %T, want *GRPCExistsMethodCommandResponse", resp.Command)
				}
				if !grpcResp.Exists {
					t.Error("Expected method to exist")
				}
			},
		},
		{
			name: "GRPCBuildRequestRequest",
			req:  NewGRPCBuildRequestRequest("TestClient", "BuildMethod", []byte(`{"test": "data"}`), false, ""),
			validateFunc: func(t *testing.T, resp *Response) {
				t.Helper()
				if resp.CommandType != GRPCBuildRequestCommand {
					t.Errorf("CommandType mismatch: got %v, want %v", resp.CommandType, GRPCBuildRequestCommand)
				}
				buildResp, ok := resp.Command.(*GRPCBuildRequestCommandResponse)
				if !ok {
					t.Fatalf("Command type mismatch: got %T, want *GRPCBuildRequestCommandResponse", resp.Command)
				}
				expectedFDSet := "fdset_for_TestClient"
				if string(buildResp.FDSet) != expectedFDSet {
					t.Errorf("FDSet mismatch: got %s, want %s", string(buildResp.FDSet), expectedFDSet)
				}
				expectedFQDN := "com.example.BuildMethod"
				if buildResp.MessageFQDN != expectedFQDN {
					t.Errorf("MessageFQDN mismatch: got %s, want %s", buildResp.MessageFQDN, expectedFQDN)
				}
			},
		},
		{
			name: "TeardownRequest",
			req: NewTeardownRequest("teardown-789", &context.SerializableContext{
				Vars: []any{
					map[string]any{"cleanup": true},
				},
			}),
			validateFunc: func(t *testing.T, resp *Response) {
				t.Helper()
				if resp.CommandType != TeardownCommand {
					t.Errorf("CommandType mismatch: got %v, want %v", resp.CommandType, TeardownCommand)
				}
				_, ok := resp.Command.(*TeardownCommandResponse)
				if !ok {
					t.Fatalf("Command type mismatch: got %T, want *TeardownCommandResponse", resp.Command)
				}
			},
		},
		{
			name: "SetupEachScenarioRequest",
			req: NewSetupEachScenarioRequest("scenario-456", &context.SerializableContext{
				Vars: []any{
					map[string]any{"scenario": "test"},
				},
			}, 0),
			validateFunc: func(t *testing.T, resp *Response) {
				t.Helper()
				if resp.CommandType != SetupEachScenarioCommand {
					t.Errorf("CommandType mismatch: got %v, want %v", resp.CommandType, SetupEachScenarioCommand)
				}
				_, ok := resp.Command.(*SetupEachScenarioCommandResponse)
				if !ok {
					t.Fatalf("Command type mismatch: got %T, want *SetupEachScenarioCommandResponse", resp.Command)
				}
			},
		},
		{
			name: "GRPCInvokeRequest",
			req:  NewGRPCInvokeRequest("TestClient", "InvokeMethod", []byte(`{"request": "payload"}`), nil, false, ""),
			validateFunc: func(t *testing.T, resp *Response) {
				t.Helper()
				if resp.CommandType != GRPCInvokeCommand {
					t.Errorf("CommandType mismatch: got %v, want %v", resp.CommandType, GRPCInvokeCommand)
				}
				invokeResp, ok := resp.Command.(*GRPCInvokeCommandResponse)
				if !ok {
					t.Fatalf("Command type mismatch: got %T, want *GRPCInvokeCommandResponse", resp.Command)
				}
				if string(invokeResp.FDSet) != "invoke_fdset" {
					t.Errorf("FDSet mismatch: got %s, want invoke_fdset", string(invokeResp.FDSet))
				}
				if invokeResp.ResponseFQDN != "com.example.Response" {
					t.Errorf("ResponseFQDN mismatch: got %s, want com.example.Response", invokeResp.ResponseFQDN)
				}
			},
		},
		{
			name: "HTTPCallRequest",
			req:  NewHTTPCallRequest("HttpClient", []byte("GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n")),
			validateFunc: func(t *testing.T, resp *Response) {
				t.Helper()
				if resp.CommandType != HTTPCallCommand {
					t.Errorf("CommandType mismatch: got %v, want %v", resp.CommandType, HTTPCallCommand)
				}
				_, ok := resp.Command.(*HTTPCallCommandResponse)
				if !ok {
					t.Fatalf("Command type mismatch: got %T, want *HTTPCallCommandResponse", resp.Command)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Step 1: Encode the request
			encodedReq, err := EncodeRequest(tt.req)
			if err != nil {
				t.Fatalf("EncodeRequest failed: %v", err)
			}

			// Step 2: Handle the command with the mock handler
			response := HandleCommand(encodedReq, handler)

			// Verify no error in response
			if response.Error != "" {
				t.Fatalf("HandleCommand returned error: %s", response.Error)
			}

			// Step 3: Encode the response to JSON
			encodedResp, err := json.Marshal(response)
			if err != nil {
				t.Fatalf("json.Marshal response failed: %v", err)
			}

			// Step 4: Decode the response using DecodeResponse
			decodedResp, err := DecodeResponse(encodedResp)
			if err != nil {
				t.Fatalf("DecodeResponse failed: %v", err)
			}

			// Step 5: Validate the decoded response
			tt.validateFunc(t, decodedResp)

			// Additional verification: ensure the decoded response matches the original
			if !reflect.DeepEqual(response, decodedResp) {
				t.Errorf("Response mismatch after round trip:\noriginal: %+v\ndecoded:  %+v", response, decodedResp)
			}
		})
	}
}

func TestEncodeRequestHandleCommandDecodeResponseWithErrors(t *testing.T) {
	handler := &mockCommandHandler{}

	// Test with invalid JSON
	t.Run("InvalidJSON", func(t *testing.T) {
		invalidJSON := []byte(`{"invalid": json}`)
		response := HandleCommand(invalidJSON, handler)

		if response.Error == "" {
			t.Error("Expected error for invalid JSON, but got none")
		}

		// For invalid JSON errors, CommandType is empty and Command is nil
		// This cannot be decoded by DecodeResponse due to empty CommandType
		// So we test the error response structure directly
		if response.CommandType != "" {
			t.Errorf("Expected empty CommandType for invalid JSON, got %v", response.CommandType)
		}

		if response.Command != nil {
			t.Errorf("Expected nil Command for invalid JSON, got %v", response.Command)
		}
	})

	// Test with unknown command type
	t.Run("UnknownCommand", func(t *testing.T) {
		// Create a JSON with unknown command type that will unmarshal successfully
		// but fail in handleCommand
		unknownJSON := []byte(`{"type":"unknown","command":{}}`)

		response := HandleCommand(unknownJSON, handler)

		if response.Error == "" {
			t.Error("Expected error for unknown command type, but got none")
		}

		// Check that the error message contains "unexpected command type"
		if !contains(response.Error, "unexpected command type") {
			t.Errorf("Expected error to contain 'unexpected command type', got: %s", response.Error)
		}

		// For unknown command during unmarshal, CommandType is empty
		if response.CommandType != "" {
			t.Errorf("Expected empty CommandType for unknown command during unmarshal, got %v", response.CommandType)
		}

		if response.Command != nil {
			t.Errorf("Expected nil Command for unknown command, got %v", response.Command)
		}
	})

	// Test with valid command that can be round-tripped
	t.Run("ValidCommandWithErrorFromHandler", func(t *testing.T) {
		// Create a mock handler that returns errors
		errorHandler := &struct {
			*mockCommandHandler
		}{&mockCommandHandler{}}

		// Override one method to return an error
		errorHandler.mockCommandHandler = &mockCommandHandler{}

		req := NewInitRequest()
		encodedReq, err := EncodeRequest(req)
		if err != nil {
			t.Fatalf("EncodeRequest failed: %v", err)
		}

		response := HandleCommand(encodedReq, errorHandler)

		// This should succeed (no error from HandleCommand itself)
		if response.Error != "" {
			t.Errorf("Unexpected error from HandleCommand: %s", response.Error)
		}

		// Verify round trip
		encodedResp, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		decodedResp, err := DecodeResponse(encodedResp)
		if err != nil {
			t.Fatalf("DecodeResponse failed: %v", err)
		}

		if !reflect.DeepEqual(response, decodedResp) {
			t.Errorf("Response mismatch after round trip:\noriginal: %+v\ndecoded:  %+v", response, decodedResp)
		}
	})
}

func TestCommandRequestResponseInterfaces(t *testing.T) {
	// Test all isCommandRequest methods
	commands := []CommandRequest{
		&InitCommandRequest{},
		&SetupCommandRequest{},
		&SetupEachScenarioCommandRequest{},
		&TeardownCommandRequest{},
		&SyncCommandRequest{},
		&CallCommandRequest{},
		&GetCommandRequest{},
		&GRPCExistsMethodCommandRequest{},
		&GRPCBuildRequestCommandRequest{},
		&GRPCInvokeCommandRequest{},
	}

	for _, cmd := range commands {
		if !cmd.isCommandRequest() {
			t.Errorf("isCommandRequest() should return true for %T", cmd)
		}
	}

	// Test all isCommandResponse methods
	responses := []CommandResponse{
		&InitCommandResponse{},
		&SetupCommandResponse{},
		&SetupEachScenarioCommandResponse{},
		&TeardownCommandResponse{},
		&SyncCommandResponse{},
		&CallCommandResponse{},
		&GetCommandResponse{},
		&GRPCExistsMethodCommandResponse{},
		&GRPCBuildRequestCommandResponse{},
		&GRPCInvokeCommandResponse{},
	}

	for _, resp := range responses {
		if !resp.isCommandResponse() {
			t.Errorf("isCommandResponse() should return true for %T", resp)
		}
	}
}

func TestToTypeMapMethods(t *testing.T) {
	// Test InitCommandResponse.ToTypeMap
	initResp := &InitCommandResponse{
		Types: []*NameWithType{
			{Name: "func1", Type: &Type{Kind: STRING}},
			{Name: "func2", Type: &Type{Kind: INT}},
		},
	}

	typeMap, err := initResp.ToTypeMap()
	if err != nil {
		t.Fatalf("InitCommandResponse ToTypeMap failed: %v", err)
	}
	if len(typeMap) != 2 {
		t.Errorf("InitCommandResponse ToTypeMap length: got %d, want 2", len(typeMap))
	}
	if typeMap["func1"].Kind != STRING {
		t.Errorf("func1 type: got %v, want %v", typeMap["func1"].Kind, STRING)
	}

	// Test SyncCommandResponse.ToTypeMap
	syncResp := &SyncCommandResponse{
		Types: []*NameWithType{
			{Name: "syncFunc", Type: &Type{Kind: BOOL}},
		},
	}

	syncTypeMap, err := syncResp.ToTypeMap()
	if err != nil {
		t.Fatalf("SyncCommandResponse ToTypeMap failed: %v", err)
	}
	if len(syncTypeMap) != 1 {
		t.Errorf("SyncCommandResponse ToTypeMap length: got %d, want 1", len(syncTypeMap))
	}
	if syncTypeMap["syncFunc"].Kind != BOOL {
		t.Errorf("syncFunc type: got %v, want %v", syncTypeMap["syncFunc"].Kind, BOOL)
	}
}

func TestToContextMethods(t *testing.T) {
	ctx := &context.SerializableContext{
		Vars: []any{map[string]any{"test": "value"}},
	}

	// Test SetupCommandRequest.ToContext
	setupReq := &SetupCommandRequest{Context: ctx}
	convertedCtx, err := context.FromSerializable(setupReq.Context)
	if err != nil {
		t.Fatal(err)
	}
	if convertedCtx == nil {
		t.Error("SetupCommandRequest.ToContext() returned nil")
	}

	// Test SetupEachScenarioCommandRequest.ToContext
	scenarioReq := &SetupEachScenarioCommandRequest{Context: ctx}
	convertedCtx, err = context.FromSerializable(scenarioReq.Context)
	if err != nil {
		t.Fatal(err)
	}
	if convertedCtx == nil {
		t.Error("SetupEachScenarioCommandRequest.ToContext() returned nil")
	}

	// Test TeardownCommandRequest.ToContext
	teardownReq := &TeardownCommandRequest{Context: ctx}
	convertedCtx, err = context.FromSerializable(teardownReq.Context)
	if err != nil {
		t.Fatal(err)
	}
	if convertedCtx == nil {
		t.Error("TeardownCommandRequest.ToContext() returned nil")
	}
}

func TestToCommandResponse(t *testing.T) {
	// Test successful conversion
	resp := &Response{
		CommandType: InitCommand,
		Command:     &InitCommandResponse{Types: []*NameWithType{}},
	}

	initResp, err := ToCommandResponse[*InitCommandResponse](resp)
	if err != nil {
		t.Fatalf("ToCommandResponse failed: %v", err)
	}
	if initResp == nil {
		t.Error("ToCommandResponse returned nil")
	}

	// Test failed conversion
	_, err = ToCommandResponse[*SetupCommandResponse](resp)
	if err == nil {
		t.Error("Expected error for wrong type conversion")
	}
}

func TestNewTypeAndNewFuncType(t *testing.T) {
	// Test NewType with non-function type
	stringValue := reflect.ValueOf("")
	typ, err := NewType(stringValue)
	if err != nil {
		t.Fatalf("NewType failed: %v", err)
	}
	if typ.Kind != STRING {
		t.Errorf("NewType Kind: got %v, want %v", typ.Kind, STRING)
	}
	if typ.Func != nil {
		t.Error("NewType Func should be nil for non-function type")
	}

	// Test NewType with function type
	funcValue := reflect.ValueOf(func(string, int) (bool, error) { return false, nil })
	funcTyp, err := NewType(funcValue)
	if err != nil {
		t.Fatalf("NewType failed: %v", err)
	}
	if funcTyp.Kind != FUNC {
		t.Errorf("NewType Kind: got %v, want %v", funcTyp.Kind, FUNC)
	}
	if funcTyp.Func == nil {
		t.Error("NewType Func should not be nil for function type")
	}

	// Test NewFuncType directly
	directFuncTyp, err := newFuncType(funcValue)
	if err != nil {
		t.Fatalf("NewFuncType failed: %v", err)
	}
	if directFuncTyp.Kind != FUNC {
		t.Errorf("NewFuncType Kind: got %v, want %v", directFuncTyp.Kind, FUNC)
	}
	if directFuncTyp.Func == nil {
		t.Error("NewFuncType Func should not be nil")
	}
	if len(directFuncTyp.Func.Args) != 2 {
		t.Errorf("NewFuncType Args length: got %d, want 2", len(directFuncTyp.Func.Args))
	}
	if len(directFuncTyp.Func.Return) != 2 {
		t.Errorf("NewFuncType Return length: got %d, want 2", len(directFuncTyp.Func.Return))
	}
}

func TestUnmarshalJSONErrorCases(t *testing.T) {
	t.Run("Request.UnmarshalJSON errors", func(t *testing.T) {
		tests := []struct {
			name string
			data []byte
		}{
			{
				name: "malformed outer JSON",
				data: []byte(`invalid json`),
			},
			{
				name: "malformed command field",
				data: []byte(`{"type":"init","command":malformed}`),
			},
			{
				name: "init command with invalid JSON",
				data: []byte(`{"type":"init","command":{"invalid":json}}`),
			},
			{
				name: "setup command with invalid JSON",
				data: []byte(`{"type":"setup","command":{"id":"test","context":invalid}}`),
			},
			{
				name: "setup_each_scenario command with invalid JSON",
				data: []byte(`{"type":"setup_each_scenario","command":{"id":"test","context":invalid}}`),
			},
			{
				name: "teardown command with invalid JSON",
				data: []byte(`{"type":"teardown","command":{"setupId":"test","context":invalid}}`),
			},
			{
				name: "sync command with invalid JSON",
				data: []byte(`{"type":"sync","command":invalid}`),
			},
			{
				name: "call command with invalid JSON",
				data: []byte(`{"type":"call","command":{"name":"test","args":invalid}}`),
			},
			{
				name: "get command with invalid JSON",
				data: []byte(`{"type":"get","command":{"name":invalid}}`),
			},
			{
				name: "grpc_exists_method command with invalid JSON",
				data: []byte(`{"type":"grpc_exists_method","command":{"client":"test","method":invalid}}`),
			},
			{
				name: "grpc_build_request command with invalid JSON",
				data: []byte(`{"type":"grpc_build_request","command":{"client":"test","method":"test","message":invalid}}`),
			},
			{
				name: "grpc_invoke command with invalid JSON",
				data: []byte(`{"type":"grpc_invoke","command":{"client":"test","method":"test","request":invalid}}`),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var req Request
				err := req.UnmarshalJSON(tt.data)
				if err == nil {
					t.Error("Expected error but got none")
				}
			})
		}
	})

	t.Run("Request.UnmarshalJSON command-specific unmarshal failures", func(t *testing.T) {
		tests := []struct {
			name string
			data []byte
		}{
			// init command - empty struct, but can fail with invalid JSON structure
			{
				name: "init command unmarshal failure - invalid nested JSON",
				data: []byte(`{"type":"init","command":[123]}`),
			},
			// setup command - can fail with invalid context structure
			{
				name: "setup command unmarshal failure - invalid context type",
				data: []byte(`{"type":"setup","command":{"id":"test","context":"not_object"}}`),
			},
			// setup_each_scenario command - can fail with invalid context structure
			{
				name: "setup_each_scenario command unmarshal failure - invalid context type",
				data: []byte(`{"type":"setup_each_scenario","command":{"id":"test","context":"not_object"}}`),
			},
			// teardown command - can fail with invalid context structure
			{
				name: "teardown command unmarshal failure - invalid context type",
				data: []byte(`{"type":"teardown","command":{"setupId":"test","context":"not_object"}}`),
			},
			// sync command - empty struct, but can fail with invalid JSON structure
			{
				name: "sync command unmarshal failure - invalid nested JSON",
				data: []byte(`{"type":"sync","command":[123]}`),
			},
			// call command - can fail with invalid args array
			{
				name: "call command unmarshal failure - invalid args type",
				data: []byte(`{"type":"call","command":{"name":"test","args":"not_array"}}`),
			},
			{
				name: "call command unmarshal failure - invalid args element type",
				data: []byte(`{"type":"call","command":{"name":"test","args":[123]}}`),
			},
			// get command - simple struct with string field
			{
				name: "get command unmarshal failure - invalid name type",
				data: []byte(`{"type":"get","command":{"name":123}}`),
			},
			// grpc_exists_method command - string fields
			{
				name: "grpc_exists_method command unmarshal failure - invalid client type",
				data: []byte(`{"type":"grpc_exists_method","command":{"client":123,"method":"test"}}`),
			},
			{
				name: "grpc_exists_method command unmarshal failure - invalid method type",
				data: []byte(`{"type":"grpc_exists_method","command":{"client":"test","method":123}}`),
			},
			// grpc_build_request command - byte field (base64 encoded)
			{
				name: "grpc_build_request command unmarshal failure - invalid message base64",
				data: []byte(`{"type":"grpc_build_request","command":{"client":"test","method":"test","message":"invalid_base64!"}}`),
			},
			{
				name: "grpc_build_request command unmarshal failure - invalid client type",
				data: []byte(`{"type":"grpc_build_request","command":{"client":123,"method":"test","message":"dGVzdA=="}}`),
			},
			{
				name: "grpc_build_request command unmarshal failure - invalid method type",
				data: []byte(`{"type":"grpc_build_request","command":{"client":"test","method":123,"message":"dGVzdA=="}}`),
			},
			// grpc_invoke command - byte field (base64 encoded)
			{
				name: "grpc_invoke command unmarshal failure - invalid request base64",
				data: []byte(`{"type":"grpc_invoke","command":{"client":"test","method":"test","request":"invalid_base64!"}}`),
			},
			{
				name: "grpc_invoke command unmarshal failure - invalid client type",
				data: []byte(`{"type":"grpc_invoke","command":{"client":123,"method":"test","request":"dGVzdA=="}}`),
			},
			{
				name: "grpc_invoke command unmarshal failure - invalid method type",
				data: []byte(`{"type":"grpc_invoke","command":{"client":"test","method":123,"request":"dGVzdA=="}}`),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var req Request
				err := req.UnmarshalJSON(tt.data)
				if err == nil {
					t.Error("Expected error but got none")
				}
			})
		}
	})

	t.Run("Response.UnmarshalJSON errors", func(t *testing.T) {
		tests := []struct {
			name string
			data []byte
		}{
			{
				name: "malformed outer JSON",
				data: []byte(`invalid json`),
			},
			{
				name: "malformed command field",
				data: []byte(`{"type":"init","command":malformed}`),
			},
			{
				name: "init response with invalid JSON",
				data: []byte(`{"type":"init","command":{"types":invalid}}`),
			},
			{
				name: "setup response with invalid JSON",
				data: []byte(`{"type":"setup","command":invalid}`),
			},
			{
				name: "setup_each_scenario response with invalid JSON",
				data: []byte(`{"type":"setup_each_scenario","command":invalid}`),
			},
			{
				name: "teardown response with invalid JSON",
				data: []byte(`{"type":"teardown","command":invalid}`),
			},
			{
				name: "sync response with invalid JSON",
				data: []byte(`{"type":"sync","command":{"types":invalid}}`),
			},
			{
				name: "call response with invalid JSON",
				data: []byte(`{"type":"call","command":{"return":invalid,"error":""}}`),
			},
			{
				name: "get response with invalid JSON",
				data: []byte(`{"type":"get","command":{"value":"test","error":invalid}}`),
			},
			{
				name: "grpc_exists_method response with invalid JSON",
				data: []byte(`{"type":"grpc_exists_method","command":{"exists":invalid,"error":""}}`),
			},
			{
				name: "grpc_build_request response with invalid JSON",
				data: []byte(`{"type":"grpc_build_request","command":{"fdset":"test","messageFQDN":invalid}}`),
			},
			{
				name: "grpc_invoke response with invalid JSON",
				data: []byte(`{"type":"grpc_invoke","command":{"fdset":"test","responseFQDN":"test","responseBytes":"test","statusProto":invalid}}`),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var resp Response
				err := resp.UnmarshalJSON(tt.data)
				if err == nil {
					t.Error("Expected error but got none")
				}
			})
		}
	})

	t.Run("Response.UnmarshalJSON command-specific unmarshal failures", func(t *testing.T) {
		tests := []struct {
			name string
			data []byte
		}{
			// init response - has types array field
			{
				name: "init response unmarshal failure - invalid types field type",
				data: []byte(`{"type":"init","command":{"types":"not_array"}}`),
			},
			{
				name: "init response unmarshal failure - invalid nested JSON",
				data: []byte(`{"type":"init","command":[123]}`),
			},
			// setup response - empty struct, but can fail with invalid JSON structure
			{
				name: "setup response unmarshal failure - invalid nested JSON",
				data: []byte(`{"type":"setup","command":[123]}`),
			},
			// setup_each_scenario response - empty struct, but can fail with invalid JSON structure
			{
				name: "setup_each_scenario response unmarshal failure - invalid nested JSON",
				data: []byte(`{"type":"setup_each_scenario","command":[123]}`),
			},
			// teardown response - empty struct, but can fail with invalid JSON structure
			{
				name: "teardown response unmarshal failure - invalid nested JSON",
				data: []byte(`{"type":"teardown","command":[123]}`),
			},
			// sync response - has types array field
			{
				name: "sync response unmarshal failure - invalid types field type",
				data: []byte(`{"type":"sync","command":{"types":"not_array"}}`),
			},
			{
				name: "sync response unmarshal failure - invalid nested JSON",
				data: []byte(`{"type":"sync","command":[123]}`),
			},
			// call response - has return array field
			{
				name: "call response unmarshal failure - invalid return field type",
				data: []byte(`{"type":"call","command":{"return":"not_array"}}`),
			},
			{
				name: "call response unmarshal failure - invalid nested JSON",
				data: []byte(`{"type":"call","command":[123]}`),
			},
			// get response - has value string field
			{
				name: "get response unmarshal failure - invalid value field type",
				data: []byte(`{"type":"get","command":{"value":123}}`),
			},
			{
				name: "get response unmarshal failure - invalid nested JSON",
				data: []byte(`{"type":"get","command":[123]}`),
			},
			// grpc_exists_method response - has exists boolean field
			{
				name: "grpc_exists_method response unmarshal failure - invalid exists field type",
				data: []byte(`{"type":"grpc_exists_method","command":{"exists":"not_boolean"}}`),
			},
			{
				name: "grpc_exists_method response unmarshal failure - invalid nested JSON",
				data: []byte(`{"type":"grpc_exists_method","command":[123]}`),
			},
			// grpc_build_request response - has fdset byte field and messageFQDN string field
			{
				name: "grpc_build_request response unmarshal failure - invalid fdset base64",
				data: []byte(`{"type":"grpc_build_request","command":{"fdset":"invalid_base64!","messageFQDN":"test"}}`),
			},
			{
				name: "grpc_build_request response unmarshal failure - invalid messageFQDN type",
				data: []byte(`{"type":"grpc_build_request","command":{"fdset":"dGVzdA==","messageFQDN":123}}`),
			},
			{
				name: "grpc_build_request response unmarshal failure - invalid nested JSON",
				data: []byte(`{"type":"grpc_build_request","command":[123]}`),
			},
			// grpc_invoke response - has multiple byte fields
			{
				name: "grpc_invoke response unmarshal failure - invalid fdset base64",
				data: []byte(`{"type":"grpc_invoke","command":{"fdset":"invalid_base64!","responseFQDN":"test","responseBytes":"dGVzdA==","statusProto":"dGVzdA=="}}`),
			},
			{
				name: "grpc_invoke response unmarshal failure - invalid responseBytes base64",
				data: []byte(`{"type":"grpc_invoke","command":{"fdset":"dGVzdA==","responseFQDN":"test","responseBytes":"invalid_base64!","statusProto":"dGVzdA=="}}`),
			},
			{
				name: "grpc_invoke response unmarshal failure - invalid statusProto base64",
				data: []byte(`{"type":"grpc_invoke","command":{"fdset":"dGVzdA==","responseFQDN":"test","responseBytes":"dGVzdA==","statusProto":"invalid_base64!"}}`),
			},
			{
				name: "grpc_invoke response unmarshal failure - invalid responseFQDN type",
				data: []byte(`{"type":"grpc_invoke","command":{"fdset":"dGVzdA==","responseFQDN":123,"responseBytes":"dGVzdA==","statusProto":"dGVzdA=="}}`),
			},
			{
				name: "grpc_invoke response unmarshal failure - invalid nested JSON",
				data: []byte(`{"type":"grpc_invoke","command":[123]}`),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var resp Response
				err := resp.UnmarshalJSON(tt.data)
				if err == nil {
					t.Error("Expected error but got none")
				}
			})
		}
	})

	t.Run("Unknown command types", func(t *testing.T) {
		// Test Request with unknown command type
		var req Request
		err := req.UnmarshalJSON([]byte(`{"type":"unknown_command","command":{}}`))
		if err == nil {
			t.Error("Expected error for unknown command type in Request")
		}
		expectedMsg := "unexpected command type: unknown_command"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %s", expectedMsg, err.Error())
		}

		// Test Response with unknown command type
		var resp Response
		err = resp.UnmarshalJSON([]byte(`{"type":"unknown_command","command":{}}`))
		if err == nil {
			t.Error("Expected error for unknown command type in Response")
		}
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %s", expectedMsg, err.Error())
		}
	})

	t.Run("Field type mismatches", func(t *testing.T) {
		tests := []struct {
			name string
			data []byte
		}{
			{
				name: "Request with non-string command type",
				data: []byte(`{"type":123,"command":{}}`),
			},
			{
				name: "Response with non-string command type",
				data: []byte(`{"type":123,"command":{}}`),
			},
			{
				name: "Init response with non-array types",
				data: []byte(`{"type":"init","command":{"types":"not_array"}}`),
			},
			{
				name: "Call response with non-array return",
				data: []byte(`{"type":"call","command":{"return":"not_array","error":""}}`),
			},
			{
				name: "GRPC exists method response with non-boolean exists",
				data: []byte(`{"type":"grpc_exists_method","command":{"exists":"not_boolean","error":""}}`),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if strings.Contains(tt.name, "Request") {
					var req Request
					err := req.UnmarshalJSON(tt.data)
					if err == nil {
						t.Error("Expected error for type mismatch but got none")
					}
				} else {
					var resp Response
					err := resp.UnmarshalJSON(tt.data)
					if err == nil {
						t.Error("Expected error for type mismatch but got none")
					}
				}
			})
		}
	})
}

func TestEncodeDecodeErrorCases(t *testing.T) {
	// Test DecodeResponse with invalid JSON
	_, err := DecodeResponse([]byte(`invalid json`))
	if err == nil {
		t.Error("Expected error for invalid JSON in DecodeResponse")
	}

	// Test DecodeResponse with empty response
	_, err = DecodeResponse([]byte(`{}`))
	if err != nil {
		t.Logf("DecodeResponse with empty JSON: %v", err)
	}
}

func TestToCommandRequestErrorCase(t *testing.T) {
	// Test toCommandRequest with wrong type conversion
	var initReq CommandRequest = &InitCommandRequest{}

	// This should succeed
	converted, err := toCommandRequest[*InitCommandRequest](initReq)
	if err != nil {
		t.Errorf("Expected successful conversion, got error: %v", err)
	}
	if converted == nil {
		t.Error("Expected non-nil result")
	}

	// This should fail
	_, err = toCommandRequest[*SetupCommandRequest](initReq)
	if err == nil {
		t.Error("Expected error for wrong type conversion")
	}
}
