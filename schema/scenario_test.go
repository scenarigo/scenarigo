package schema

import (
	"testing"
	"time"

	"github.com/scenarigo/scenarigo/assert"
	"github.com/scenarigo/scenarigo/context"
)

func TestStep_ToCurrentStep(t *testing.T) {
	mockInvoker := &mockInvoker{}
	mockAssertionBuilder := &mockAssertionBuilder{}

	timeout := Duration(5 * time.Second)
	maxRetries := int(3)
	postTimeoutWaitingLimit := Duration(10 * time.Second)

	step := &Step{
		ID:              "test-step-id",
		Title:           "Test Step Title",
		Description:     "This is a test step description",
		If:              "{{vars.condition}}",
		ContinueOnError: true,
		Vars:            map[string]any{"var1": "value1", "var2": 123},
		Secrets:         map[string]any{"secret1": "secretValue1"},
		Protocol:        "http",
		Request:         mockInvoker,
		Expect:          mockAssertionBuilder,
		Include:         "include-file.yaml",
		Ref:             "{{plugins.testPlugin}}",
		Bind: Bind{
			Vars:    map[string]any{"bindVar1": "bindValue1"},
			Secrets: map[string]any{"bindSecret1": "bindSecretValue1"},
		},
		Timeout:                 &timeout,
		PostTimeoutWaitingLimit: &postTimeoutWaitingLimit,
		Retry: &RetryPolicy{
			Constant: &RetryPolicyConstant{
				Interval:   (*Duration)(&timeout),
				MaxRetries: &maxRetries,
			},
		},
	}

	idx := 5
	currentStep := step.ToCurrentStep(idx)

	if currentStep.Index != idx {
		t.Errorf("Index: expected %d, got %d", idx, currentStep.Index)
	}
	if currentStep.ID != step.ID {
		t.Errorf("ID: expected %q, got %q", step.ID, currentStep.ID)
	}
	if currentStep.Title != step.Title {
		t.Errorf("Title: expected %q, got %q", step.Title, currentStep.Title)
	}
	if currentStep.Description != step.Description {
		t.Errorf("Description: expected %q, got %q", step.Description, currentStep.Description)
	}
	if currentStep.If != step.If {
		t.Errorf("If: expected %q, got %q", step.If, currentStep.If)
	}
	if currentStep.ContinueOnError != step.ContinueOnError {
		t.Errorf("ContinueOnError: expected %v, got %v", step.ContinueOnError, currentStep.ContinueOnError)
	}
	if len(currentStep.Vars) != len(step.Vars) {
		t.Errorf("Vars length: expected %d, got %d", len(step.Vars), len(currentStep.Vars))
	}
	for k, v := range step.Vars {
		if currentStep.Vars[k] != v {
			t.Errorf("Vars[%q]: expected %v, got %v", k, v, currentStep.Vars[k])
		}
	}
	if len(currentStep.Secrets) != len(step.Secrets) {
		t.Errorf("Secrets length: expected %d, got %d", len(step.Secrets), len(currentStep.Secrets))
	}
	for k, v := range step.Secrets {
		if currentStep.Secrets[k] != v {
			t.Errorf("Secrets[%q]: expected %v, got %v", k, v, currentStep.Secrets[k])
		}
	}
	if currentStep.Protocol != step.Protocol {
		t.Errorf("Protocol: expected %q, got %q", step.Protocol, currentStep.Protocol)
	}
	if currentStep.Request != step.Request {
		t.Errorf("Request: expected %v, got %v", step.Request, currentStep.Request)
	}
	if currentStep.Expect != step.Expect {
		t.Errorf("Expect: expected %v, got %v", step.Expect, currentStep.Expect)
	}
	if currentStep.Include != step.Include {
		t.Errorf("Include: expected %q, got %q", step.Include, currentStep.Include)
	}
	if currentStep.Ref != step.Ref {
		t.Errorf("Ref: expected %v, got %v", step.Ref, currentStep.Ref)
	}
	if len(currentStep.BindVars) != len(step.Bind.Vars) {
		t.Errorf("BindVars length: expected %d, got %d", len(step.Bind.Vars), len(currentStep.BindVars))
	}
	for k, v := range step.Bind.Vars {
		if currentStep.BindVars[k] != v {
			t.Errorf("BindVars[%q]: expected %v, got %v", k, v, currentStep.BindVars[k])
		}
	}
	if len(currentStep.BindSecrets) != len(step.Bind.Secrets) {
		t.Errorf("BindSecrets length: expected %d, got %d", len(step.Bind.Secrets), len(currentStep.BindSecrets))
	}
	for k, v := range step.Bind.Secrets {
		if currentStep.BindSecrets[k] != v {
			t.Errorf("BindSecrets[%q]: expected %v, got %v", k, v, currentStep.BindSecrets[k])
		}
	}
	if currentStep.Timeout == nil || *currentStep.Timeout != time.Duration(timeout) {
		t.Errorf("Timeout: expected %v, got %v", time.Duration(timeout), currentStep.Timeout)
	}
	if currentStep.PostTimeoutWaitingLimit == nil || *currentStep.PostTimeoutWaitingLimit != time.Duration(postTimeoutWaitingLimit) {
		t.Errorf("PostTimeoutWaitingLimit: expected %v, got %v", time.Duration(postTimeoutWaitingLimit), currentStep.PostTimeoutWaitingLimit)
	}
	if currentStep.Retry != step.Retry {
		t.Errorf("Retry: expected %v, got %v", step.Retry, currentStep.Retry)
	}
}

type mockInvoker struct{}

func (m *mockInvoker) Invoke(ctx *context.Context) (*context.Context, any, error) {
	return ctx, nil, nil
}

type mockAssertionBuilder struct{}

func (m *mockAssertionBuilder) Build(ctx *context.Context) (assert.Assertion, error) {
	return nil, nil
}
