package padlockcloud

import "fmt"
import "net/http"

func JsonifyErrorResponse(e ErrorResponse) []byte {
	return []byte(fmt.Sprintf("{\"error\":\"%s\",\"message\":\"%s\"}", e.Code(), e.Message()))
}

type ErrorResponse interface {
	error
	Code() string
	Status() int
	Message() string
}

type BadRequest struct {
	Msg string
}

func (e *BadRequest) Code() string {
	return "bad_request"
}

func (e *BadRequest) Error() string {
	return fmt.Sprintf("%s - %s", e.Code(), e.Msg)
}

func (e *BadRequest) Status() int {
	return http.StatusBadRequest
}

func (e *BadRequest) Message() string {
	return fmt.Sprintf("%s: %s", http.StatusText(e.Status()), e.Msg)
}

type InvalidAuthToken struct {
	email string
	token string
}

func (e *InvalidAuthToken) Code() string {
	return "invalid_auth_token"
}

func (e *InvalidAuthToken) Error() string {
	return fmt.Sprintf("%s - %s:%s", e.Code(), e.email, e.token)
}

func (e *InvalidAuthToken) Status() int {
	return http.StatusUnauthorized
}

func (e *InvalidAuthToken) Message() string {
	return fmt.Sprintf("%s - %s", http.StatusText(e.Status()), "No valid authorization token provided")
}

type ExpiredAuthToken struct {
	email string
	token string
}

func (e *ExpiredAuthToken) Code() string {
	return "expired_auth_token"
}

func (e *ExpiredAuthToken) Error() string {
	return fmt.Sprintf("%s - %s:%s", e.Code(), e.email, e.token)
}

func (e *ExpiredAuthToken) Status() int {
	return http.StatusUnauthorized
}

func (e *ExpiredAuthToken) Message() string {
	return fmt.Sprintf("%s - %s", http.StatusText(e.Status()), "The provided authorization token has expired")
}

type InvalidCsrfToken struct {
	reason error
}

func (e *InvalidCsrfToken) Code() string {
	return "invalid_csrf_token"
}

func (e *InvalidCsrfToken) Error() string {
	return fmt.Sprintf("%s - %s", e.Code(), e.reason)
}

func (e *InvalidCsrfToken) Status() int {
	return http.StatusForbidden
}

func (e *InvalidCsrfToken) Message() string {
	return fmt.Sprintf("%s - %s", http.StatusText(e.Status()), "Invalid CSRF Token")
}

type MethodNotAllowed struct {
	method string
}

func (e *MethodNotAllowed) Code() string {
	return "method_not_allowed"
}

func (e *MethodNotAllowed) Error() string {
	return fmt.Sprintf("%s - %s", e.Code(), e.method)
}

func (e *MethodNotAllowed) Status() int {
	return http.StatusMethodNotAllowed
}

func (e *MethodNotAllowed) Message() string {
	return http.StatusText(e.Status())
}

type UnsupportedEndpoint struct {
	path string
}

func (e *UnsupportedEndpoint) Code() string {
	return "unsupported_endpoint"
}

func (e *UnsupportedEndpoint) Error() string {
	return fmt.Sprintf("%s - %s", e.Code(), e.path)
}

func (e *UnsupportedEndpoint) Status() int {
	return http.StatusNotFound
}

func (e *UnsupportedEndpoint) Message() string {
	return http.StatusText(e.Status())
}

type AccountNotFound struct {
	email string
}

func (e *AccountNotFound) Code() string {
	return "account_not_found"
}

func (e *AccountNotFound) Error() string {
	return fmt.Sprintf("%s - %s", e.Code(), e.email)
}

func (e *AccountNotFound) Status() int {
	return http.StatusNotFound
}

func (e *AccountNotFound) Message() string {
	return http.StatusText(e.Status())
}

type UnsupportedApiVersion struct {
	found    int
	expected int
}

func (e *UnsupportedApiVersion) Code() string {
	return "deprecated_api_version"
}

func (e *UnsupportedApiVersion) Error() string {
	return fmt.Sprintf("%s - %d!=%d", e.Code(), e.found, e.expected)
}

func (e *UnsupportedApiVersion) Status() int {
	return http.StatusNotAcceptable
}

func (e *UnsupportedApiVersion) Message() string {
	return fmt.Sprintf("The api version you are using (%d) is not supported. Please use version %d", e.found, e.expected)
}

type RateLimitExceeded struct {
}

func (e *RateLimitExceeded) Code() string {
	return "rate_limit_exceeded"
}

func (e *RateLimitExceeded) Error() string {
	return fmt.Sprintf("%s", e.Code())
}

func (e *RateLimitExceeded) Status() int {
	return http.StatusTooManyRequests
}

func (e *RateLimitExceeded) Message() string {
	return http.StatusText(e.Status())
}

type ServerError struct {
	error
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("%s - %v", e.Code(), e.error)
}

func (e *ServerError) Code() string {
	return "internal_server_error"
}

func (e *ServerError) Status() int {
	return http.StatusInternalServerError
}

func (e *ServerError) Message() string {
	return "Something went wrong on our side, sorry! Our team has been notified and will resolve the problem as soon as possible!"
}

func (e *ServerError) Format(s fmt.State, verb rune) {
	if err, ok := e.error.(fmt.Formatter); ok {
		err.Format(s, verb)
	} else {
		fmt.Fprintf(s, "%"+string(verb), e.error)
	}
}

type UnauthorizedError struct {
}

func (e *UnauthorizedError) Code() string {
	return "unauthorized"
}

func (e *UnauthorizedError) Error() string {
	return fmt.Sprintf("%s", e.Code())
}

func (e *UnauthorizedError) Status() int {
	return http.StatusUnauthorized
}

func (e *UnauthorizedError) Message() string {
	return fmt.Sprintf("%s - %s", http.StatusText(e.Status()), "You are not authorized to view this page.")
}
