package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/kubot64/conflux/internal/apperror"
)

// Writer は stdout/stderr への出力を管理する。
// JSON フラグに応じて JSON またはプレーンテキストで出力する。
type Writer struct {
	JSON bool
	Out  io.Writer // デフォルト os.Stdout
	Err  io.Writer // デフォルト os.Stderr
}

// New は Writer を生成する。
func New(jsonMode bool) *Writer {
	return &Writer{
		JSON: jsonMode,
		Out:  os.Stdout,
		Err:  os.Stderr,
	}
}

type envelope struct {
	SchemaVersion int    `json:"schema_version"`
	Command       string `json:"command"`
	Result        any    `json:"result,omitempty"`
	Errors        any    `json:"errors,omitempty"`
	Error         any    `json:"error,omitempty"`
	Warning       any    `json:"warning,omitempty"`
}

// Write は result を stdout に書き出す（JSON or テキスト）。
func (w *Writer) Write(command string, result any) error {
	if w.JSON {
		return json.NewEncoder(w.Out).Encode(envelope{
			SchemaVersion: 1,
			Command:       command,
			Result:        result,
		})
	}
	_, err := fmt.Fprintln(w.Out, result)
	return err
}

// WriteWithErrors は result と errors[] を同時に stdout に書き出す（部分失敗時用）。
// errors が空の場合は通常の Write と同じ挙動になる。
func (w *Writer) WriteWithErrors(command string, result any, errors any) error {
	if w.JSON {
		return json.NewEncoder(w.Out).Encode(envelope{
			SchemaVersion: 1,
			Command:       command,
			Result:        result,
			Errors:        errors,
		})
	}
	_, err := fmt.Fprintln(w.Out, result)
	return err
}

// WriteError はエラーを stderr に書き出す（JSON or テキスト）。
func (w *Writer) WriteError(command string, err error) {
	if w.JSON {
		var appErr *apperror.AppError
		var errPayload map[string]any
		if e, ok := err.(*apperror.AppError); ok {
			appErr = e
			errPayload = map[string]any{
				"code":    int(appErr.Code()),
				"kind":    string(appErr.Kind),
				"message": appErr.Message,
			}
		} else {
			errPayload = map[string]any{
				"code":    1,
				"kind":    string(apperror.KindValidation),
				"message": err.Error(),
			}
		}
		_ = json.NewEncoder(w.Err).Encode(envelope{
			SchemaVersion: 1,
			Command:       command,
			Error:         errPayload,
		})
		return
	}
	fmt.Fprintf(w.Err, "error: %v\n", err)
}

// WriteWarning は警告を stderr に書き出す（JSON or テキスト）。
func (w *Writer) WriteWarning(command string, kind string, message string) {
	if w.JSON {
		_ = json.NewEncoder(w.Err).Encode(envelope{
			SchemaVersion: 1,
			Command:       command,
			Warning: map[string]any{
				"kind":    kind,
				"message": message,
			},
		})
		return
	}
	fmt.Fprintf(w.Err, "warning [%s]: %s\n", kind, message)
}
