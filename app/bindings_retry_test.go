package app

import (
	"strings"
	"testing"
)

func TestRetrySubmoduleSetup_SessionNotFound(t *testing.T) {
	api := newTestAPI(t)

	err := api.RetrySubmoduleSetup("does-not-exist", []string{"x"})
	if err == nil {
		t.Fatal("expected error for non-existent session, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to mention 'not found', got: %v", err)
	}
}
