package apperror

// ErrorKind は apperror の種別を表す。
type ErrorKind string

const (
	KindValidation ErrorKind = "validation_error"
	KindAuth       ErrorKind = "auth_error"
	KindServer     ErrorKind = "server_error"
	KindTimeout    ErrorKind = "timeout"
	KindCanceled   ErrorKind = "canceled"
	KindNotFound   ErrorKind = "not_found"
	KindConflict   ErrorKind = "conflict"
)

// ExitCode はプロセスの終了コードを表す。
type ExitCode int

const (
	ExitOK         ExitCode = 0
	ExitValidation ExitCode = 1
	ExitAuth       ExitCode = 2
	ExitServer     ExitCode = 3
	ExitNotFound   ExitCode = 4
	ExitConflict   ExitCode = 5
)

// AppError はアプリケーションエラーを表す。
type AppError struct {
	Kind    ErrorKind
	Message string
}

func (e *AppError) Error() string { return e.Message }

// Code は ErrorKind に対応する ExitCode を返す。
func (e *AppError) Code() ExitCode {
	switch e.Kind {
	case KindValidation:
		return ExitValidation
	case KindAuth:
		return ExitAuth
	case KindServer, KindTimeout, KindCanceled:
		return ExitServer
	case KindNotFound:
		return ExitNotFound
	case KindConflict:
		return ExitConflict
	default:
		return ExitOK
	}
}

// New は AppError を生成する。
func New(kind ErrorKind, message string) *AppError {
	return &AppError{Kind: kind, Message: message}
}
