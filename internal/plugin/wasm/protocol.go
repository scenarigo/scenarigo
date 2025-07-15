package wasm

import (
	"encoding/json"
	"fmt"

	"github.com/scenarigo/scenarigo/context"
)

// Command represents a WASM plugin command type.
type Command string

const (
	InitCommand              Command = "init"
	SetupCommand             Command = "setup"
	SetupEachScenarioCommand Command = "setup_each_scenario"
	TeardownCommand          Command = "teardown"
	SyncCommand              Command = "sync"
	CallCommand              Command = "call"
	MethodCommand            Command = "method"
	GetCommand               Command = "get"
	GRPCExistsMethodCommand  Command = "grpc_exists_method"
	GRPCBuildRequestCommand  Command = "grpc_build_request"
	GRPCInvokeCommand        Command = "grpc_invoke"
)

// Request represents a command request sent to WASM plugins.
type Request struct {
	CommandType Command        `json:"type"`
	Command     CommandRequest `json:"command"`
}

// Response represents a command response from WASM plugins.
type Response struct {
	CommandType Command         `json:"type"`
	Command     CommandResponse `json:"command"`
	Error       string          `json:"error"`
}

// CommandRequest is an interface for all command request types.
type CommandRequest interface {
	isCommandRequest() bool
}

// CommandResponse is an interface for all command response types.
type CommandResponse interface {
	isCommandResponse() bool
}

var (
	_ CommandRequest = new(InitCommandRequest)
	_ CommandRequest = new(SetupCommandRequest)
	_ CommandRequest = new(SetupEachScenarioCommandRequest)
	_ CommandRequest = new(TeardownCommandRequest)
	_ CommandRequest = new(SyncCommandRequest)
	_ CommandRequest = new(CallCommandRequest)
	_ CommandRequest = new(MethodCommandRequest)
	_ CommandRequest = new(GetCommandRequest)
	_ CommandRequest = new(GRPCExistsMethodCommandRequest)
	_ CommandRequest = new(GRPCBuildRequestCommandRequest)
	_ CommandRequest = new(GRPCInvokeCommandRequest)
)

var (
	_ CommandResponse = new(InitCommandResponse)
	_ CommandResponse = new(SetupCommandResponse)
	_ CommandResponse = new(SetupEachScenarioCommandResponse)
	_ CommandResponse = new(TeardownCommandResponse)
	_ CommandResponse = new(SyncCommandResponse)
	_ CommandResponse = new(CallCommandResponse)
	_ CommandResponse = new(MethodCommandResponse)
	_ CommandResponse = new(GetCommandResponse)
	_ CommandResponse = new(GRPCExistsMethodCommandResponse)
	_ CommandResponse = new(GRPCBuildRequestCommandResponse)
	_ CommandResponse = new(GRPCInvokeCommandResponse)
)

// NewInitRequest creates a new initialization request.
func NewInitRequest() *Request {
	return &Request{
		CommandType: InitCommand,
		Command:     &InitCommandRequest{},
	}
}

// NewSetupRequest creates a new setup request with context.
func NewSetupRequest(setupID string, ctx *context.SerializableContext) *Request {
	return &Request{
		CommandType: SetupCommand,
		Command: &SetupCommandRequest{
			ID:      setupID,
			Context: ctx,
		},
	}
}

// NewSyncRequest creates a new synchronization request.
func NewSyncRequest() *Request {
	return &Request{
		CommandType: SyncCommand,
		Command:     &SyncCommandRequest{},
	}
}

// NewTeardownRequest creates a new teardown request with context.
func NewTeardownRequest(setupID string, ctx *context.SerializableContext) *Request {
	return &Request{
		CommandType: TeardownCommand,
		Command: &TeardownCommandRequest{
			SetupID: setupID,
			Context: ctx,
		},
	}
}

// NewSetupEachScenarioRequest creates a new setup request for each scenario.
func NewSetupEachScenarioRequest(setupID string, ctx *context.SerializableContext) *Request {
	return &Request{
		CommandType: SetupEachScenarioCommand,
		Command: &SetupEachScenarioCommandRequest{
			ID:      setupID,
			Context: ctx,
		},
	}
}

// NewCallRequest creates a new function call request.
func NewCallRequest(name string, selectors []string, args []string) *Request {
	return &Request{
		CommandType: CallCommand,
		Command: &CallCommandRequest{
			Name:      name,
			Selectors: selectors,
			Args:      args,
		},
	}
}

// NewMethodRequest creates a new function method request.
func NewMethodRequest(name string, selectors []string) *Request {
	return &Request{
		CommandType: MethodCommand,
		Command: &MethodCommandRequest{
			Name:      name,
			Selectors: selectors,
		},
	}
}

// NewGetRequest creates a new value get request.
func NewGetRequest(name string, selectors []string) *Request {
	return &Request{
		CommandType: GetCommand,
		Command: &GetCommandRequest{
			Name:      name,
			Selectors: selectors,
		},
	}
}

// NewGRPCExistsMethodRequest creates a request to check if a gRPC method exists.
func NewGRPCExistsMethodRequest(client, method string) *Request {
	return &Request{
		CommandType: GRPCExistsMethodCommand,
		Command: &GRPCExistsMethodCommandRequest{
			Client: client,
			Method: method,
		},
	}
}

// NewGRPCBuildRequestRequest creates a request to build a gRPC request message.
func NewGRPCBuildRequestRequest(client, method string, msg []byte) *Request {
	return &Request{
		CommandType: GRPCBuildRequestCommand,
		Command: &GRPCBuildRequestCommandRequest{
			Client:  client,
			Method:  method,
			Message: msg,
		},
	}
}

// NewGRPCInvokeRequest creates a request to invoke a gRPC method.
func NewGRPCInvokeRequest(client, method string, reqMsg []byte) *Request {
	return &Request{
		CommandType: GRPCInvokeCommand,
		Command: &GRPCInvokeCommandRequest{
			Client:  client,
			Method:  method,
			Request: reqMsg,
		},
	}
}

type InitCommandRequest struct{}

func (r *InitCommandRequest) isCommandRequest() bool { return true }

// InitCommandResponse contains the types available from the WASM plugin.
type InitCommandResponse struct {
	TypeRefMap map[string]*Type `json:"typeRefMap"`
	Types      []*NameWithType  `json:"types"`
}

// ToTypeMap converts the response types to a name-to-type mapping.
func (r *InitCommandResponse) ToTypeMap() (map[string]*Type, error) {
	nameToTypeMap := make(map[string]*Type)
	for _, typ := range r.Types {
		resolvedType, err := ResolveRef(typ.Type, r.TypeRefMap)
		if err != nil {
			return nil, err
		}
		nameToTypeMap[typ.Name] = resolvedType
	}
	return nameToTypeMap, nil
}

func (r *InitCommandResponse) isCommandResponse() bool { return true }

type SetupCommandRequest struct {
	ID      string                       `json:"id"`
	Context *context.SerializableContext `json:"context"`
}

func (r *SetupCommandRequest) ToContext() *context.Context {
	return context.FromSerializable(r.Context)
}

func (r *SetupCommandRequest) isCommandRequest() bool { return true }

type SetupCommandResponse struct{}

func (r *SetupCommandResponse) isCommandResponse() bool { return true }

type SetupEachScenarioCommandRequest struct {
	ID      string                       `json:"id"`
	Context *context.SerializableContext `json:"context"`
}

func (r *SetupEachScenarioCommandRequest) ToContext() *context.Context {
	return context.FromSerializable(r.Context)
}

func (r *SetupEachScenarioCommandRequest) isCommandRequest() bool { return true }

type SetupEachScenarioCommandResponse struct{}

func (r *SetupEachScenarioCommandResponse) isCommandResponse() bool { return true }

type TeardownCommandRequest struct {
	SetupID string                       `json:"setupId"`
	Context *context.SerializableContext `json:"context"`
}

func (r *TeardownCommandRequest) ToContext() *context.Context {
	return context.FromSerializable(r.Context)
}

func (r *TeardownCommandRequest) isCommandRequest() bool { return true }

type TeardownCommandResponse struct{}

func (r *TeardownCommandResponse) isCommandResponse() bool { return true }

type SyncCommandRequest struct{}

func (r *SyncCommandRequest) isCommandRequest() bool { return true }

// SyncCommandResponse contains updated types from the WASM plugin.
type SyncCommandResponse struct {
	TypeRefMap map[string]*Type `json:"typeRefMap"`
	Types      []*NameWithType  `json:"types"`
}

// ToTypeMap converts the response types to a name-to-type mapping.
func (r *SyncCommandResponse) ToTypeMap() (map[string]*Type, error) {
	nameToTypeMap := make(map[string]*Type)
	for _, typ := range r.Types {
		resolvedType, err := ResolveRef(typ.Type, r.TypeRefMap)
		if err != nil {
			return nil, err
		}
		nameToTypeMap[typ.Name] = resolvedType
	}
	return nameToTypeMap, nil
}

func (r *SyncCommandResponse) isCommandResponse() bool { return true }

type CallCommandRequest struct {
	Name      string   `json:"name"`
	Selectors []string `json:"selectors"`
	Args      []string `json:"args"`
}

func (r *CallCommandRequest) isCommandRequest() bool { return true }

type CallCommandResponse struct {
	Return []*ReturnValue `json:"return"`
}

func (r *CallCommandResponse) isCommandResponse() bool { return true }

type MethodCommandRequest struct {
	Name      string   `json:"name"`
	Selectors []string `json:"selectors"`
}

func (r *MethodCommandRequest) isCommandRequest() bool { return true }

type MethodCommandResponse struct {
	TypeRefMap map[string]*Type `json:"typeRefMap"`
	Type       *Type            `json:"type"`
}

func (r *MethodCommandResponse) isCommandResponse() bool { return true }

type GetCommandRequest struct {
	Name      string   `json:"name"`
	Selectors []string `json:"selectors"`
}

func (r *GetCommandRequest) isCommandRequest() bool { return true }

type GetCommandResponse struct {
	Value string `json:"value"`
}

func (r *GetCommandResponse) isCommandResponse() bool { return true }

type GRPCExistsMethodCommandRequest struct {
	Client string `json:"client"`
	Method string `json:"method"`
}

func (r *GRPCExistsMethodCommandRequest) isCommandRequest() bool { return true }

type GRPCExistsMethodCommandResponse struct {
	Exists bool `json:"exists"`
}

func (r *GRPCExistsMethodCommandResponse) isCommandResponse() bool { return true }

type GRPCBuildRequestCommandRequest struct {
	Client  string `json:"client"`
	Method  string `json:"method"`
	Message []byte `json:"message"`
}

func (r *GRPCBuildRequestCommandRequest) isCommandRequest() bool { return true }

type GRPCBuildRequestCommandResponse struct {
	FDSet       []byte `json:"fdset"`
	MessageFQDN string `json:"messageFQDN"` //nolint:tagliatelle
}

func (r *GRPCBuildRequestCommandResponse) isCommandResponse() bool { return true }

type GRPCInvokeCommandRequest struct {
	Client  string `json:"client"`
	Method  string `json:"method"`
	Request []byte `json:"request"`
}

func (r *GRPCInvokeCommandRequest) isCommandRequest() bool { return true }

type GRPCInvokeCommandResponse struct {
	FDSet         []byte `json:"fdset"`
	ResponseFQDN  string `json:"responseFQDN"` //nolint:tagliatelle
	ResponseBytes []byte `json:"responseBytes"`
	StatusProto   []byte `json:"statusProto"`
}

func (r *GRPCInvokeCommandResponse) isCommandResponse() bool { return true }

// ReturnValue represents a return value from a WASM plugin function call.
type ReturnValue struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

func (r *Request) UnmarshalJSON(b []byte) error {
	var req struct {
		CommandType Command         `json:"type"`
		Command     json.RawMessage `json:"command"`
	}
	if err := json.Unmarshal(b, &req); err != nil {
		return err
	}
	r.CommandType = req.CommandType
	switch req.CommandType {
	case InitCommand:
		var v InitCommandRequest
		if err := json.Unmarshal(req.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case SetupCommand:
		var v SetupCommandRequest
		if err := json.Unmarshal(req.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case SetupEachScenarioCommand:
		var v SetupEachScenarioCommandRequest
		if err := json.Unmarshal(req.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case TeardownCommand:
		var v TeardownCommandRequest
		if err := json.Unmarshal(req.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case SyncCommand:
		var v SyncCommandRequest
		if err := json.Unmarshal(req.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case CallCommand:
		var v CallCommandRequest
		if err := json.Unmarshal(req.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case MethodCommand:
		var v MethodCommandRequest
		if err := json.Unmarshal(req.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case GetCommand:
		var v GetCommandRequest
		if err := json.Unmarshal(req.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case GRPCExistsMethodCommand:
		var v GRPCExistsMethodCommandRequest
		if err := json.Unmarshal(req.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case GRPCBuildRequestCommand:
		var v GRPCBuildRequestCommandRequest
		if err := json.Unmarshal(req.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case GRPCInvokeCommand:
		var v GRPCInvokeCommandRequest
		if err := json.Unmarshal(req.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	}
	return fmt.Errorf("unexpected command type: %s", req.CommandType)
}

func (r *Response) UnmarshalJSON(b []byte) error {
	var res struct {
		CommandType Command         `json:"type"`
		Command     json.RawMessage `json:"command"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return err
	}
	r.CommandType = res.CommandType
	switch res.CommandType {
	case InitCommand:
		var v InitCommandResponse
		if err := json.Unmarshal(res.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case SetupCommand:
		var v SetupCommandResponse
		if err := json.Unmarshal(res.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case SetupEachScenarioCommand:
		var v SetupEachScenarioCommandResponse
		if err := json.Unmarshal(res.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case TeardownCommand:
		var v TeardownCommandResponse
		if err := json.Unmarshal(res.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case SyncCommand:
		var v SyncCommandResponse
		if err := json.Unmarshal(res.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case CallCommand:
		var v CallCommandResponse
		if err := json.Unmarshal(res.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case MethodCommand:
		var v MethodCommandResponse
		if err := json.Unmarshal(res.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case GetCommand:
		var v GetCommandResponse
		if err := json.Unmarshal(res.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case GRPCExistsMethodCommand:
		var v GRPCExistsMethodCommandResponse
		if err := json.Unmarshal(res.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case GRPCBuildRequestCommand:
		var v GRPCBuildRequestCommandResponse
		if err := json.Unmarshal(res.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	case GRPCInvokeCommand:
		var v GRPCInvokeCommandResponse
		if err := json.Unmarshal(res.Command, &v); err != nil {
			return err
		}
		r.Command = &v
		return nil
	}
	return fmt.Errorf("unexpected command type: %s", res.CommandType)
}

// CommandHandler defines the interface for handling WASM plugin commands.
type CommandHandler interface {
	Init(*InitCommandRequest) (*InitCommandResponse, error)
	Setup(*SetupCommandRequest) (*SetupCommandResponse, error)
	SetupEachScenario(*SetupEachScenarioCommandRequest) (*SetupEachScenarioCommandResponse, error)
	Teardown(*TeardownCommandRequest) (*TeardownCommandResponse, error)
	Sync(*SyncCommandRequest) (*SyncCommandResponse, error)
	Call(*CallCommandRequest) (*CallCommandResponse, error)
	Method(*MethodCommandRequest) (*MethodCommandResponse, error)
	Get(*GetCommandRequest) (*GetCommandResponse, error)
	GRPCExistsMethod(*GRPCExistsMethodCommandRequest) (*GRPCExistsMethodCommandResponse, error)
	GRPCBuildRequest(*GRPCBuildRequestCommandRequest) (*GRPCBuildRequestCommandResponse, error)
	GRPCInvoke(*GRPCInvokeCommandRequest) (*GRPCInvokeCommandResponse, error)
}

// HandleCommand processes a command from WASM plugin and returns a response.
func HandleCommand(b []byte, handler CommandHandler) *Response {
	var r Request
	if err := json.Unmarshal(b, &r); err != nil {
		return &Response{Error: err.Error()}
	}
	cmd, err := handleCommand(&r, handler)
	if err != nil {
		return &Response{CommandType: r.CommandType, Error: err.Error()}
	}
	return &Response{CommandType: r.CommandType, Command: cmd}
}

func handleCommand(r *Request, handler CommandHandler) (CommandResponse, error) {
	switch r.CommandType {
	case InitCommand:
		cmd, err := toCommandRequest[*InitCommandRequest](r.Command)
		if err != nil {
			return nil, err
		}
		return handler.Init(cmd)
	case SetupCommand:
		cmd, err := toCommandRequest[*SetupCommandRequest](r.Command)
		if err != nil {
			return nil, err
		}
		return handler.Setup(cmd)
	case SetupEachScenarioCommand:
		cmd, err := toCommandRequest[*SetupEachScenarioCommandRequest](r.Command)
		if err != nil {
			return nil, err
		}
		return handler.SetupEachScenario(cmd)
	case TeardownCommand:
		cmd, err := toCommandRequest[*TeardownCommandRequest](r.Command)
		if err != nil {
			return nil, err
		}
		return handler.Teardown(cmd)
	case SyncCommand:
		cmd, err := toCommandRequest[*SyncCommandRequest](r.Command)
		if err != nil {
			return nil, err
		}
		return handler.Sync(cmd)
	case CallCommand:
		cmd, err := toCommandRequest[*CallCommandRequest](r.Command)
		if err != nil {
			return nil, err
		}
		return handler.Call(cmd)
	case MethodCommand:
		cmd, err := toCommandRequest[*MethodCommandRequest](r.Command)
		if err != nil {
			return nil, err
		}
		return handler.Method(cmd)
	case GetCommand:
		cmd, err := toCommandRequest[*GetCommandRequest](r.Command)
		if err != nil {
			return nil, err
		}
		return handler.Get(cmd)
	case GRPCExistsMethodCommand:
		cmd, err := toCommandRequest[*GRPCExistsMethodCommandRequest](r.Command)
		if err != nil {
			return nil, err
		}
		return handler.GRPCExistsMethod(cmd)
	case GRPCBuildRequestCommand:
		cmd, err := toCommandRequest[*GRPCBuildRequestCommandRequest](r.Command)
		if err != nil {
			return nil, err
		}
		return handler.GRPCBuildRequest(cmd)
	case GRPCInvokeCommand:
		cmd, err := toCommandRequest[*GRPCInvokeCommandRequest](r.Command)
		if err != nil {
			return nil, err
		}
		return handler.GRPCInvoke(cmd)
	}
	return nil, fmt.Errorf("unknown command type: %s", r.CommandType)
}

func toCommandRequest[T CommandRequest](v CommandRequest) (T, error) {
	vv, ok := v.(T)
	if !ok {
		var ret T
		return ret, fmt.Errorf("failed to convert %T command request type from %T", ret, v)
	}
	return vv, nil
}

// ToCommandResponse converts a response to a specific command response type.
func ToCommandResponse[T CommandResponse](res *Response) (T, error) {
	v, ok := res.Command.(T)
	if !ok {
		var ret T
		return ret, fmt.Errorf("failed to convert %T comand response type from %T", ret, res.Command)
	}
	return v, nil
}

// EncodeRequest encodes a request for transmission to WASM plugin.
func EncodeRequest(r *Request) ([]byte, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// DecodeResponse decodes a response from WASM plugin.
func DecodeResponse(b []byte) (*Response, error) {
	var res Response
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
