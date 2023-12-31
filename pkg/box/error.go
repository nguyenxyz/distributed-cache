package box

type ErrorType string

const (
	KeyNotFound      ErrorType = "KeyNotFound"
	InvalidOperation           = "InvalidOperation"
	Unauthorized               = "Unauthorized"
)

type OperationError struct {
	err     error
	errType ErrorType
}

func (e OperationError) Error() string {
	return e.err.Error()
}

func (e OperationError) Type() ErrorType {
	return e.errType
}
