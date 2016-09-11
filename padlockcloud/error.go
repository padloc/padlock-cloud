package padlockcloud

import "fmt"
import "net/http"
import "net/http/httputil"

func formatRequest(r *http.Request) string {
	ip := r.Header.Get("X-Real-IP")
	if ip == "" {
		ip = r.RemoteAddr
	}
	return fmt.Sprintf("%s %s %s", ip, r.Method, r.URL)
}

func formatRequestVerbose(r *http.Request) string {
	dump, _ := httputil.DumpRequest(r, true)
	return string(dump)
}

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
}

func (e *BadRequest) Code() string {
	return "bad_request"
}

func (e *BadRequest) Error() string {
	return fmt.Sprintf("%s", e.Code())
}

func (e *BadRequest) Status() int {
	return http.StatusBadRequest
}

func (e *BadRequest) Message() string {
	return http.StatusText(e.Status())
}

type InvalidToken struct {
	token string
}

func (e *InvalidToken) Code() string {
	return "invalid_token"
}

func (e *InvalidToken) Error() string {
	return fmt.Sprintf("%s: %s", e.Code(), e.token)
}

func (e *InvalidToken) Status() int {
	return http.StatusBadRequest
}

func (e *InvalidToken) Message() string {
	return "Invalid Token"
}

type Unauthorized struct {
	email string
	token string
}

func (e *Unauthorized) Code() string {
	return "unauthorized"
}

func (e *Unauthorized) Error() string {
	return fmt.Sprintf("%s: %s:%s", e.Code(), e.email, e.token)
}

func (e *Unauthorized) Status() int {
	return http.StatusUnauthorized
}

func (e *Unauthorized) Message() string {
	return http.StatusText(e.Status())
}

type MethodNotAllowed struct {
	method string
}

func (e *MethodNotAllowed) Code() string {
	return "method_not_allowed"
}

func (e *MethodNotAllowed) Error() string {
	return fmt.Sprintf("%s: %s", e.Code(), e.method)
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
	return fmt.Sprintf("%s: %s", e.Code(), e.path)
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
	return fmt.Sprintf("%s: %s", e.Code(), e.email)
}

func (e *AccountNotFound) Status() int {
	return http.StatusNotFound
}

func (e *AccountNotFound) Message() string {
	return http.StatusText(e.Status())
}

type UnsupportedApiVersion struct {
	version int
}

func (e *UnsupportedApiVersion) Code() string {
	return "deprecated_api_version"
}

func (e *UnsupportedApiVersion) Error() string {
	return fmt.Sprintf("%s: %d", e.Code(), e.version)
}

func (e *UnsupportedApiVersion) Status() int {
	return http.StatusNotAcceptable
}

func (e *UnsupportedApiVersion) Message() string {
	return fmt.Sprintf("The api version you are using (%d) is not supported. Please use version %d", e.version, ApiVersion)
}

type TooManyRequests struct {
}

func (e *TooManyRequests) Code() string {
	return "too_many_requests"
}

func (e *TooManyRequests) Error() string {
	return fmt.Sprintf("%s", e.Code())
}

func (e *TooManyRequests) Status() int {
	return http.StatusTooManyRequests
}

func (e *TooManyRequests) Message() string {
	return http.StatusText(e.Status())
}

type ServerError struct {
	error
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code(), e.error.Error())
}

func (e *ServerError) Code() string {
	return "internal_server_error"
}

func (e *ServerError) Status() int {
	return http.StatusInternalServerError
}

func (e *ServerError) Message() string {
	return http.StatusText(e.Status())
}
