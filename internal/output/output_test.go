package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/output"
)

// --- JSON モード ---

func TestWrite_JSON(t *testing.T) {
	var out, errBuf bytes.Buffer
	w := output.New(true)
	w.Out = &out
	w.Err = &errBuf

	result := map[string]any{"ok": true}
	if err := w.Write("ping", result); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out.String())
	}
	if got["schema_version"] != float64(1) {
		t.Errorf("schema_version: got %v", got["schema_version"])
	}
	if got["command"] != "ping" {
		t.Errorf("command: got %v", got["command"])
	}
	if _, ok := got["result"]; !ok {
		t.Error("result field missing")
	}
}

func TestWriteError_JSON(t *testing.T) {
	var out, errBuf bytes.Buffer
	w := output.New(true)
	w.Out = &out
	w.Err = &errBuf

	appErr := apperror.New(apperror.KindNotFound, "page 12345 not found")
	w.WriteError("page get", appErr)

	var got map[string]any
	if err := json.Unmarshal(errBuf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON on stderr: %v\nraw: %s", err, errBuf.String())
	}
	if got["schema_version"] != float64(1) {
		t.Errorf("schema_version: got %v", got["schema_version"])
	}
	if got["command"] != "page get" {
		t.Errorf("command: got %v", got["command"])
	}
	errField, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field missing or wrong type")
	}
	if errField["code"] != float64(4) {
		t.Errorf("code: got %v, want 4", errField["code"])
	}
	if errField["kind"] != "not_found" {
		t.Errorf("kind: got %v", errField["kind"])
	}
	if errField["message"] != "page 12345 not found" {
		t.Errorf("message: got %v", errField["message"])
	}
}

func TestWriteWarning_JSON(t *testing.T) {
	var out, errBuf bytes.Buffer
	w := output.New(true)
	w.Out = &out
	w.Err = &errBuf

	w.WriteWarning("page create", "history_write_failed", "permission denied")

	var got map[string]any
	if err := json.Unmarshal(errBuf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON on stderr: %v\nraw: %s", err, errBuf.String())
	}
	if got["schema_version"] != float64(1) {
		t.Errorf("schema_version: got %v", got["schema_version"])
	}
	warnField, ok := got["warning"].(map[string]any)
	if !ok {
		t.Fatalf("warning field missing")
	}
	if warnField["kind"] != "history_write_failed" {
		t.Errorf("kind: got %v", warnField["kind"])
	}
	if warnField["message"] != "permission denied" {
		t.Errorf("message: got %v", warnField["message"])
	}
}

// --- 非 JSON モード ---

func TestWrite_Text(t *testing.T) {
	var out, errBuf bytes.Buffer
	w := output.New(false)
	w.Out = &out
	w.Err = &errBuf

	if err := w.Write("ping", "pong"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(out.String(), "pong") {
		t.Errorf("expected 'pong' in output, got: %s", out.String())
	}
}

func TestWriteError_Text(t *testing.T) {
	var out, errBuf bytes.Buffer
	w := output.New(false)
	w.Out = &out
	w.Err = &errBuf

	appErr := apperror.New(apperror.KindAuth, "unauthorized")
	w.WriteError("ping", appErr)

	if !strings.Contains(errBuf.String(), "unauthorized") {
		t.Errorf("expected error message in stderr, got: %s", errBuf.String())
	}
}

func TestWriteWarning_Text(t *testing.T) {
	var out, errBuf bytes.Buffer
	w := output.New(false)
	w.Out = &out
	w.Err = &errBuf

	w.WriteWarning("page create", "history_write_failed", "some error")

	if !strings.Contains(errBuf.String(), "some error") {
		t.Errorf("expected warning message in stderr, got: %s", errBuf.String())
	}
}
