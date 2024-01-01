package box

import "errors"

type ErrorType string

const (
	KeyNotFound      ErrorType = "KeyNotFound"
	InvalidOperation           = "InvalidOperation"
	Unauthorized               = "Unauthorized"
	TTLExpired                 = "TTLExpired"
	Operational                = "Operational"
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

func NewOperationError(errMsg string, errType ErrorType) *OperationError {
	return &OperationError{
		err:     errors.New(errMsg),
		errType: errType,
	}
}
