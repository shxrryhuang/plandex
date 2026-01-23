package hooks

import (
	"testing"

	shared "plandex-shared"
)

func TestRegisterHook(t *testing.T) {
	// Clear hooks before test
	originalHooks := hooks
	hooks = make(map[string]Hook)
	defer func() { hooks = originalHooks }()

	called := false
	testHook := func(params HookParams) (HookResult, *shared.ApiError) {
		called = true
		return HookResult{}, nil
	}

	RegisterHook("test_hook", testHook)

	if _, exists := hooks["test_hook"]; !exists {
		t.Error("RegisterHook did not register the hook")
	}

	// Execute to verify it's callable
	_, _ = ExecHook("test_hook", HookParams{})
	if !called {
		t.Error("registered hook was not called")
	}
}

func TestExecHookNotRegistered(t *testing.T) {
	// Clear hooks before test
	originalHooks := hooks
	hooks = make(map[string]Hook)
	defer func() { hooks = originalHooks }()

	result, err := ExecHook("nonexistent_hook", HookParams{})

	if err != nil {
		t.Errorf("ExecHook() returned error for nonexistent hook: %v", err)
	}

	// Result should be empty HookResult
	if result.GetIntegratedModelsResult != nil {
		t.Error("ExecHook() returned non-nil GetIntegratedModelsResult for nonexistent hook")
	}
	if result.ApiOrgsById != nil {
		t.Error("ExecHook() returned non-nil ApiOrgsById for nonexistent hook")
	}
	if result.FastApplyResult != nil {
		t.Error("ExecHook() returned non-nil FastApplyResult for nonexistent hook")
	}
}

func TestExecHookWithResult(t *testing.T) {
	// Clear hooks before test
	originalHooks := hooks
	hooks = make(map[string]Hook)
	defer func() { hooks = originalHooks }()

	expectedResult := HookResult{
		GetIntegratedModelsResult: &GetIntegratedModelsResult{
			IntegratedModelsMode: true,
			AuthVars:             map[string]string{"key": "value"},
		},
	}

	RegisterHook("result_hook", func(params HookParams) (HookResult, *shared.ApiError) {
		return expectedResult, nil
	})

	result, err := ExecHook("result_hook", HookParams{})

	if err != nil {
		t.Errorf("ExecHook() returned error: %v", err)
	}

	if result.GetIntegratedModelsResult == nil {
		t.Fatal("ExecHook() returned nil GetIntegratedModelsResult")
	}

	if !result.GetIntegratedModelsResult.IntegratedModelsMode {
		t.Error("IntegratedModelsMode = false, want true")
	}

	if result.GetIntegratedModelsResult.AuthVars["key"] != "value" {
		t.Errorf("AuthVars[key] = %q, want %q", result.GetIntegratedModelsResult.AuthVars["key"], "value")
	}
}

func TestExecHookWithError(t *testing.T) {
	// Clear hooks before test
	originalHooks := hooks
	hooks = make(map[string]Hook)
	defer func() { hooks = originalHooks }()

	expectedError := &shared.ApiError{
		Type:   "test_error",
		Status: 500,
		Msg:    "test error message",
	}

	RegisterHook("error_hook", func(params HookParams) (HookResult, *shared.ApiError) {
		return HookResult{}, expectedError
	})

	_, err := ExecHook("error_hook", HookParams{})

	if err == nil {
		t.Fatal("ExecHook() returned nil error, expected error")
	}

	if err.Type != expectedError.Type {
		t.Errorf("error Type = %q, want %q", err.Type, expectedError.Type)
	}
	if err.Status != expectedError.Status {
		t.Errorf("error Status = %d, want %d", err.Status, expectedError.Status)
	}
	if err.Msg != expectedError.Msg {
		t.Errorf("error Msg = %q, want %q", err.Msg, expectedError.Msg)
	}
}

func TestExecHookWithParams(t *testing.T) {
	// Clear hooks before test
	originalHooks := hooks
	hooks = make(map[string]Hook)
	defer func() { hooks = originalHooks }()

	var receivedParams HookParams

	RegisterHook("params_hook", func(params HookParams) (HookResult, *shared.ApiError) {
		receivedParams = params
		return HookResult{}, nil
	})

	testParams := HookParams{
		WillSendModelRequestParams: &WillSendModelRequestParams{
			InputTokens:  100,
			OutputTokens: 50,
			IsUserPrompt: true,
		},
	}

	_, _ = ExecHook("params_hook", testParams)

	if receivedParams.WillSendModelRequestParams == nil {
		t.Fatal("hook did not receive WillSendModelRequestParams")
	}

	if receivedParams.WillSendModelRequestParams.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", receivedParams.WillSendModelRequestParams.InputTokens)
	}
	if receivedParams.WillSendModelRequestParams.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", receivedParams.WillSendModelRequestParams.OutputTokens)
	}
	if !receivedParams.WillSendModelRequestParams.IsUserPrompt {
		t.Error("IsUserPrompt = false, want true")
	}
}

func TestHookConstants(t *testing.T) {
	constants := []struct {
		name  string
		value string
	}{
		{"HealthCheck", HealthCheck},
		{"CreateAccount", CreateAccount},
		{"WillCreatePlan", WillCreatePlan},
		{"WillTellPlan", WillTellPlan},
		{"WillExecPlan", WillExecPlan},
		{"WillSendModelRequest", WillSendModelRequest},
		{"DidSendModelRequest", DidSendModelRequest},
		{"DidFinishBuilderRun", DidFinishBuilderRun},
		{"CreateOrg", CreateOrg},
		{"Authenticate", Authenticate},
		{"GetIntegratedModels", GetIntegratedModels},
		{"GetApiOrgs", GetApiOrgs},
		{"CallFastApply", CallFastApply},
	}

	for _, c := range constants {
		t.Run(c.name, func(t *testing.T) {
			if c.value == "" {
				t.Errorf("%s constant is empty", c.name)
			}
		})
	}
}

func TestRegisterHookOverwrite(t *testing.T) {
	// Clear hooks before test
	originalHooks := hooks
	hooks = make(map[string]Hook)
	defer func() { hooks = originalHooks }()

	firstCalled := false
	secondCalled := false

	RegisterHook("overwrite_test", func(params HookParams) (HookResult, *shared.ApiError) {
		firstCalled = true
		return HookResult{}, nil
	})

	RegisterHook("overwrite_test", func(params HookParams) (HookResult, *shared.ApiError) {
		secondCalled = true
		return HookResult{}, nil
	})

	_, _ = ExecHook("overwrite_test", HookParams{})

	if firstCalled {
		t.Error("first hook was called after being overwritten")
	}
	if !secondCalled {
		t.Error("second hook was not called")
	}
}

func TestMultipleHooksIndependent(t *testing.T) {
	// Clear hooks before test
	originalHooks := hooks
	hooks = make(map[string]Hook)
	defer func() { hooks = originalHooks }()

	hook1Called := false
	hook2Called := false

	RegisterHook("hook1", func(params HookParams) (HookResult, *shared.ApiError) {
		hook1Called = true
		return HookResult{}, nil
	})

	RegisterHook("hook2", func(params HookParams) (HookResult, *shared.ApiError) {
		hook2Called = true
		return HookResult{}, nil
	})

	_, _ = ExecHook("hook1", HookParams{})

	if !hook1Called {
		t.Error("hook1 was not called")
	}
	if hook2Called {
		t.Error("hook2 was called when only hook1 should have been")
	}

	hook1Called = false
	_, _ = ExecHook("hook2", HookParams{})

	if hook1Called {
		t.Error("hook1 was called when only hook2 should have been")
	}
	if !hook2Called {
		t.Error("hook2 was not called")
	}
}

func TestFastApplyParams(t *testing.T) {
	params := FastApplyParams{
		InitialCode:       "function hello() {}",
		EditSnippet:       "function hello() { return 'world'; }",
		InitialCodeTokens: 10,
		EditSnippetTokens: 15,
		Language:          shared.LanguageJavascript,
	}

	if params.InitialCode != "function hello() {}" {
		t.Errorf("InitialCode = %q, want %q", params.InitialCode, "function hello() {}")
	}
	if params.EditSnippet != "function hello() { return 'world'; }" {
		t.Errorf("EditSnippet mismatch")
	}
	if params.InitialCodeTokens != 10 {
		t.Errorf("InitialCodeTokens = %d, want 10", params.InitialCodeTokens)
	}
	if params.EditSnippetTokens != 15 {
		t.Errorf("EditSnippetTokens = %d, want 15", params.EditSnippetTokens)
	}
	if params.Language != shared.LanguageJavascript {
		t.Errorf("Language = %v, want JavaScript", params.Language)
	}
}

func TestFastApplyResult(t *testing.T) {
	result := FastApplyResult{
		MergedCode: "merged code content",
	}

	if result.MergedCode != "merged code content" {
		t.Errorf("MergedCode = %q, want %q", result.MergedCode, "merged code content")
	}
}

func TestHookResultWithFastApply(t *testing.T) {
	// Clear hooks before test
	originalHooks := hooks
	hooks = make(map[string]Hook)
	defer func() { hooks = originalHooks }()

	RegisterHook(CallFastApply, func(params HookParams) (HookResult, *shared.ApiError) {
		if params.FastApplyParams == nil {
			return HookResult{}, &shared.ApiError{Msg: "FastApplyParams is nil"}
		}
		return HookResult{
			FastApplyResult: &FastApplyResult{
				MergedCode: params.FastApplyParams.InitialCode + " // modified",
			},
		}, nil
	})

	result, err := ExecHook(CallFastApply, HookParams{
		FastApplyParams: &FastApplyParams{
			InitialCode: "original code",
		},
	})

	if err != nil {
		t.Fatalf("ExecHook() error = %v", err)
	}

	if result.FastApplyResult == nil {
		t.Fatal("FastApplyResult is nil")
	}

	expected := "original code // modified"
	if result.FastApplyResult.MergedCode != expected {
		t.Errorf("MergedCode = %q, want %q", result.FastApplyResult.MergedCode, expected)
	}
}
