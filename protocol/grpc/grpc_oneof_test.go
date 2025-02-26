//nolint:stylecheck,tagliatelle
package grpc

type OneofMessage struct {
	Value isOneofMessage_Value `protobuf_oneof:"value"`
}

type isOneofMessage_Value interface {
	isOneofMessage_Value()
}

type OneofMessage_A_ struct {
	A *OneofMessage_A `protobuf:"bytes,1,opt,name=a,proto3,oneof"`
}

type OneofMessage_B_ struct {
	B *OneofMessage_B `protobuf:"bytes,2,opt,name=b,proto3,oneof"`
}

func (*OneofMessage_A_) isOneofMessage_Value() {}

func (*OneofMessage_B_) isOneofMessage_Value() {}

type OneofMessage_A struct {
	FooValue string `json:"foo_value,omitempty" protobuf:"bytes,1,opt,name=foo_value,json=fooValue,proto3"`
}

type OneofMessage_B struct {
	BarValue string `json:"bar_value,omitempty" protobuf:"bytes,1,opt,name=bar_value,json=barValue,proto3"`
}
