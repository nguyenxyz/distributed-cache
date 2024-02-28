package apiserver

import "net/http"

type HTTPError struct {
	Error   error
	Status  int
	Message string
}

type ErrorResponse struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

func (e *HTTPError) Payload() *ErrorResponse {
	return &ErrorResponse{
		Title:  http.StatusText(e.Status),
		Status: e.Status,
		Detail: e.Message,
	}
}

func NewBadRequestError(err error) *HTTPError {
	return &HTTPError{
		Error:   err,
		Status:  http.StatusBadRequest,
		Message: err.Error(),
	}
}

func NewInternalError(err error) *HTTPError {
	return &HTTPError{
		Error:   err,
		Status:  http.StatusInternalServerError,
		Message: "Internal Error 🥲",
	}
}
