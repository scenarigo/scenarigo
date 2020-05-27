package grpc

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/zoncoen/scenarigo/assert"
	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/internal/reflectutil"
	"github.com/zoncoen/yaml"
)

// Expect represents expected response values.
type Expect struct {
	Code     string                          `yaml:"code"`
	Body     yaml.KeyOrderPreservedInterface `yaml:"body"`
	Status   ExpectStatus                    `yaml:"status"`
	Metadata *Metadata                       `yaml:"metadata"`
}

// ExpectStatus represents expected gRPC status.
type ExpectStatus struct {
	Code    string                     `yaml:"code"`
	Message string                     `yaml:"message"`
	Details []map[string]yaml.MapSlice `yaml:"details"`
}

// Metadata represents expected gRPC metadata.
type Metadata struct {
	Header  map[string]interface{} `yaml:"header"`
	Trailer map[string]interface{} `yaml:"trailer"`
}

// Build implements protocol.AssertionBuilder interface.
func (e *Expect) Build(ctx *context.Context) (assert.Assertion, error) {
	expectBody, err := ctx.ExecuteTemplate(e.Body)
	if err != nil {
		return nil, errors.Errorf("invalid expect response: %s", err)
	}
	assertion := assert.Build(expectBody)

	return assert.AssertionFunc(func(v interface{}) error {
		message, stErr, err := extract(v)
		if err != nil {
			return err
		}
		resp, ok := v.(response)
		if !ok {
			return errors.Errorf(`failed to convert to response type. type is %s`, reflect.TypeOf(v))
		}
		if err := e.assertMetadata(resp.Header, resp.Trailer); err != nil {
			return err
		}
		if err := e.assertStatusCode(stErr); err != nil {
			return err
		}
		if err := e.assertStatusMessage(stErr); err != nil {
			return err
		}
		if err := e.assertStatusDetails(stErr); err != nil {
			return err
		}
		if err := assertion.Assert(message); err != nil {
			return err
		}
		return nil
	}), nil
}

func (e *Expect) metadataMapKeys(data metadata.MD) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	return keys
}

func (e *Expect) metadataValuesToMap(values []string) map[string]struct{} {
	valueMap := map[string]struct{}{}
	for _, value := range values {
		valueMap[value] = struct{}{}
	}
	return valueMap
}

func (e *Expect) assertMetadata(header, trailer metadata.MD) error {
	if e.Metadata == nil {
		return nil
	}
	if len(e.Metadata.Header) > 0 {
		expectedHeaderMap, err := reflectutil.ConvertStringsMap(reflect.ValueOf(e.Metadata.Header))
		if err != nil {
			return errors.Errorf(`failed to convert strings map from expected header of metadata %v`, e.Metadata.Header)
		}
		for expectedKey, expectedValues := range expectedHeaderMap {
			values := header.Get(expectedKey)
			if len(values) == 0 {
				return errors.Errorf(
					`expected metadata.header.%s is not found. actual keys are %v`,
					expectedKey, e.metadataMapKeys(header),
				)
			}
			valueMap := e.metadataValuesToMap(values)
			for _, expectedValue := range expectedValues {
				if _, exists := valueMap[expectedValue]; !exists {
					return errors.Errorf(
						`expected metadata.header.%s.%s is not found. actual values are %v`,
						expectedKey, expectedValue, values,
					)
				}
			}
		}
	}
	if len(e.Metadata.Trailer) > 0 {
		expectedTrailerMap, err := reflectutil.ConvertStringsMap(reflect.ValueOf(e.Metadata.Trailer))
		if err != nil {
			return errors.Errorf(`failed to convert strings map from expected trailer of metadata`)
		}
		for expectedKey, expectedValues := range expectedTrailerMap {
			values := trailer.Get(expectedKey)
			if len(values) == 0 {
				return errors.Errorf(
					`expected metadata.trailer.%s is not found. actual keys are %v`,
					expectedKey, e.metadataMapKeys(trailer),
				)
			}
			valueMap := e.metadataValuesToMap(values)
			for _, expectedValue := range expectedValues {
				if _, exists := valueMap[expectedValue]; !exists {
					return errors.Errorf(
						`expected metadata.trailer.%s.%s is not found. actual values are %v`,
						expectedKey, expectedValue, values,
					)
				}
			}
		}
	}
	return nil
}

func (e *Expect) assertStatusCode(sts *status.Status) error {
	expectedCode := "OK"
	if e.Code != "" {
		expectedCode = e.Code
	}
	if e.Status.Code != "" {
		expectedCode = e.Status.Code
	}

	if got, expected := sts.Code().String(), expectedCode; got == expected {
		return nil
	}
	if got, expected := strconv.Itoa(int(int32(sts.Code()))), expectedCode; got == expected {
		return nil
	}

	return errors.Errorf(`expected code is "%s" but got "%s": message="%s": details=[ %s ]`, expectedCode, sts.Code().String(), sts.Message(), detailsString(sts))
}

func (e *Expect) assertStatusMessage(sts *status.Status) error {
	if e.Status.Message == "" {
		return nil
	}

	if sts.Message() == e.Status.Message {
		return nil
	}

	return errors.Errorf(`expected status.message is "%s" but got "%s": code="%s": details=[ %s ]`, e.Status.Message, sts.Message(), sts.Code().String(), detailsString(sts))
}

func (e *Expect) assertStatusDetails(sts *status.Status) error {
	if len(e.Status.Details) == 0 {
		return nil
	}

	actualDetails := sts.Details()

	for i, expecteDetailMap := range e.Status.Details {
		if i >= len(actualDetails) {
			return errors.Errorf(`expected status.details[%d] is not found: details=[ %s ]`, i, detailsString(sts))
		}

		if len(expecteDetailMap) != 1 {
			return errors.Errorf("invalid yaml: expect status.details[%d]:"+
				"An element of status.details list must be a map of size 1 with the detail message name as the key and the value as the detail message object.", i)
		}

		var expectName string
		var expectDetail interface{}
		for k, v := range expecteDetailMap {
			expectName = k
			expectDetail = v
			break
		}

		actual, ok := actualDetails[i].(proto.Message)
		if !ok {
			return errors.Errorf(`expected status.details[%d] is "%s" but got detail is not a proto message: "%#v"`, i, expectName, actualDetails[i])
		}

		if name := proto.MessageName(actual); name != expectName {
			return errors.Errorf(`expected status.details[%d] is "%s" but got detail is "%s": details=[ %s ]`, i, expectName, name, detailsString(sts))
		}

		if err := assert.Build(expectDetail).Assert(actual); err != nil {
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

func extract(v interface{}) (proto.Message, *status.Status, error) {
	resp, ok := v.(response)
	if !ok {
		return nil, nil, errors.Errorf(`failed to convert to response type. type is %s`, reflect.TypeOf(v))
	}
	vs := resp.rvalues
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
	var sts *status.Status
	if ok {
		sts, ok = status.FromError(callErr)
		if !ok {
			return nil, nil, errors.Errorf(`expected error is status but got %T: "%s"`, callErr, callErr.Error())
		}
	}

	return message, sts, nil
}
