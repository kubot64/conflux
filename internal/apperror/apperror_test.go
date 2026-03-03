package apperror_test

import (
	"testing"

	"github.com/kubot64/conflux/internal/apperror"
)

func TestAppError_Error(t *testing.T) {
	err := &apperror.AppError{Kind: apperror.KindNotFound, Message: "page not found"}
	if err.Error() != "page not found" {
		t.Fatalf("got %q, want %q", err.Error(), "page not found")
	}
}

func TestAppError_Code(t *testing.T) {
	tests := []struct {
		kind apperror.ErrorKind
		want apperror.ExitCode
	}{
		{apperror.KindValidation, apperror.ExitValidation},
		{apperror.KindAuth, apperror.ExitAuth},
		{apperror.KindServer, apperror.ExitServer},
		{apperror.KindTimeout, apperror.ExitServer},
		{apperror.KindCanceled, apperror.ExitServer},
		{apperror.KindNotFound, apperror.ExitNotFound},
		{apperror.KindConflict, apperror.ExitConflict},
	}
	for _, tt := range tests {
		e := &apperror.AppError{Kind: tt.kind, Message: "msg"}
		if got := e.Code(); got != tt.want {
			t.Errorf("kind=%q: got code %d, want %d", tt.kind, got, tt.want)
		}
	}
}

func TestExitCodeValues(t *testing.T) {
	if apperror.ExitOK != 0 {
		t.Errorf("ExitOK must be 0")
	}
	if apperror.ExitValidation != 1 {
		t.Errorf("ExitValidation must be 1")
	}
	if apperror.ExitAuth != 2 {
		t.Errorf("ExitAuth must be 2")
	}
	if apperror.ExitServer != 3 {
		t.Errorf("ExitServer must be 3")
	}
	if apperror.ExitNotFound != 4 {
		t.Errorf("ExitNotFound must be 4")
	}
	if apperror.ExitConflict != 5 {
		t.Errorf("ExitConflict must be 5")
	}
}
