package grpc

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"google.golang.org/grpc/status"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/protocol"
	"github.com/zoncoen/yaml"
)

// Expect represents expected response values.
type Expect struct {
	Code   string                          `yaml:"code"`
	Body   yaml.KeyOrderPreservedInterface `yaml:"body"`
	Status ExpectStatus                    `yaml:"status"`
}

// ExpectStatus represents expected gRPC status
type ExpectStatus struct {
	Code    string          `yaml:"code"`
	Message string          `yaml:"message"`
	Details []yaml.MapSlice `yaml:"details"`
}

// Build implements protocol.AssertionBuilder interface.
func (e *Expect) Build(ctx *context.Context) (assert.Assertion, error) {
	expectBody, err := ctx.ExecuteTemplate(e.Body)
	if err != nil {
		return nil, errors.Errorf("invalid expect response: %s", err)
	}
	assertion := protocol.CreateAssertion(expectBody)

	return assert.AssertionFunc(func(v interface{}) error {
		message, callErr, err := extract(v)
		if err != nil {
			return err
		}
		if err := e.assertStatusCode(callErr); err != nil {
			return err
		}
		if err := e.assertStatusMessage(callErr); err != nil {
			return err
		}
		if err := e.assertStatusDetails(callErr); err != nil {
			return err
		}
		if err := assertion.Assert(message); err != nil {
			return err
		}
		return nil
	}), nil
}

func (e *Expect) assertStatusCode(stErr error) error {
	expectedCode := "OK"
	if e.Code != "" {
		expectedCode = e.Code
	}
	if e.Status.Code != "" {
		expectedCode = e.Status.Code
	}

	sts, ok := status.FromError(stErr)
	if !ok {
		return errors.Errorf(`expected code is "%s" but got non status error: %T "%s"`, expectedCode, stErr, stErr.Error())
	}

	if got, expected := sts.Code().String(), expectedCode; got == expected {
		return nil
	}
	if got, expected := strconv.Itoa(int(int32(sts.Code()))), expectedCode; got == expected {
		return nil
	}

	return errors.Errorf(`expected code is "%s" but got "%s": message="%s": details=[ %s ]`, expectedCode, sts.Code().String(), sts.Message(), detailsString(sts))
}

func (e *Expect) assertStatusMessage(stErr error) error {
	if e.Status.Message == "" {
		return nil
	}

	sts, ok := status.FromError(stErr)
	if !ok {
		return errors.Errorf(`expected status.message is "%s" but got non status error: %T "%s"`, e.Status.Message, stErr, stErr.Error())
	}

	if sts.Message() == e.Status.Message {
		return nil
	}

	return errors.Errorf(`expected status.message is "%s" but got "%s": code="%s": details=[ %s ]`, e.Status.Message, sts.Message(), sts.Code().String(), detailsString(sts))
}

func (e *Expect) assertStatusDetails(stErr error) error {
	if len(e.Status.Details) == 0 {
		return nil
	}

	sts, ok := status.FromError(stErr)
	if !ok {
		return errors.Errorf(`expected status.message is "%s" but got non status error: %T "%s"`, e.Status.Message, stErr, stErr.Error())
	}

	actualDetails := sts.Details()

	for i, mapSlice := range e.Status.Details {
		if i >= len(actualDetails) {
			return errors.Errorf(`expected status.details[%d] is not found: details=[ %s ]`, i, detailsString(sts))
		}

		if len(mapSlice) != 1 {
			return errors.Errorf("invalid yaml: expect status.details[%d]:"+
				"An element of status.details list must be a map of size 1 with the detail message name as the key and the value as the detail message object.", i)
		}

		expect := mapSlice[0]
		expectName, ok := expect.Key.(string)
		if !ok {
			return errors.Errorf("invalid yaml: expect status.details[%d]: A key of status.details[%d] must be a string protobuf message name", i, i)
		}

		actual, ok := actualDetails[i].(proto.Message)
		if !ok {
			return errors.Errorf(`expected status.details[%d] is "%s" but got detail is not a proto message: "%#v"`, i, expectName, actualDetails[i])
		}

		if name := proto.MessageName(actual); name != expectName {
			return errors.Errorf(`expected status.details[%d] is "%s" but got detail is "%s": details=[ %s ]`, i, expectName, name, detailsString(sts))
		}

		if err := protocol.CreateAssertion(expect.Value).Assert(actual); err != nil {
			return err
		}
	}

	return nil
}

func detailsString(sts *status.Status) string {
	format := "%s: {%s}"
	var details []string

	for _, i := range sts.Details() {
		if pb, ok := i.(proto.Message); ok {
			details = append(details, fmt.Sprintf(format, proto.MessageName(pb), pb.String()))
			continue
		}

		if e, ok := i.(interface{ Error() string }); ok {
			details = append(details, fmt.Sprintf(format, "<non proto message>", e.Error()))
			continue
		}

		details = append(details, fmt.Sprintf(format, "<non proto message>", fmt.Sprintf("{%#v}", i)))
	}

	return strings.Join(details, ", ")
}

func extract(v interface{}) (proto.Message, error, error) {
	vs, ok := v.([]reflect.Value)
	if !ok {
		return nil, nil, errors.Errorf("expected []reflect.Value but got %T", v)
	}
	if len(vs) != 2 {
		return nil, nil, errors.Errorf("expected return value length of method call is 2 but %d", len(vs))
	}

	if !vs[0].IsValid() {
		return nil, nil, errors.New("first return value is invalid")
	}
	message, ok := vs[0].Interface().(proto.Message)
	if !ok {
		if !vs[0].IsNil() {
			return nil, nil, errors.Errorf("expected first return value is proto.Message but %T", vs[0].Interface())
		}
	}

	if !vs[1].IsValid() {
		return nil, nil, errors.New("second return value is invalid")
	}
	callErr, ok := vs[1].Interface().(error)
	if !ok {
		if !vs[1].IsNil() {
			return nil, nil, errors.Errorf("expected second return value is error but %T", vs[1].Interface())
		}
	}

	return message, callErr, nil
}
