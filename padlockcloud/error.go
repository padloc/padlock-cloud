package padlockcloud

import "fmt"
import "net/http"

func formatRequest(r *http.Request) string {
	return fmt.Sprintf("%s %s", r.Method, r.URL)
}

type ErrorResponse interface {
	error
	Code() string
	Status() int
	Message() string
}

type BadRequest struct {
	request *http.Request
}

func (e *BadRequest) Code() string {
	return "bad_request"
}

func (e *BadRequest) Error() string {
	return fmt.Sprintf("%s - Request: %s", e.Code(), formatRequest(e.request))
}

func (e *BadRequest) Status() int {
	return http.StatusBadRequest
}

func (e *BadRequest) Message() string {
	return http.StatusText(e.Status())
}

type InvalidToken struct {
	token   string
	request *http.Request
}

func (e *InvalidToken) Code() string {
	return "invalid_token"
}

func (e *InvalidToken) Error() string {
	return fmt.Sprintf("%s - Token: %s; Request: %s", e.Code(), e.token, formatRequest(e.request))
}

func (e *InvalidToken) Status() int {
	return http.StatusBadRequest
}

func (e *InvalidToken) Message() string {
	return "Invalid Token"
}

type Unauthorized struct {
	email   string
	token   string
	request *http.Request
}

func (e *Unauthorized) Code() string {
	return "unauthorized"
}

func (e *Unauthorized) Error() string {
	return fmt.Sprintf("%s - Email: %s; Token: %s; Request: %s", e.Code(), e.email, e.token, formatRequest(e.request))
}

func (e *Unauthorized) Status() int {
	return http.StatusUnauthorized
}

func (e *Unauthorized) Message() string {
	return http.StatusText(e.Status())
}

type MethodNotAllowed struct {
	request *http.Request
}

func (e *MethodNotAllowed) Code() string {
	return "method_not_allowed"
}

func (e *MethodNotAllowed) Error() string {
	return fmt.Sprintf("%s - Request: %s", e.Code(), formatRequest(e.request))
}

func (e *MethodNotAllowed) Status() int {
	return http.StatusMethodNotAllowed
}

func (e *MethodNotAllowed) Message() string {
	return http.StatusText(e.Status())
}

type InsecureConnection struct {
	request *http.Request
}

func (e *InsecureConnection) Code() string {
	return "insecure_connection"
}

func (e *InsecureConnection) Error() string {
	return fmt.Sprintf("%s - Request: %s", e.Code(), formatRequest(e.request))
}

func (e *InsecureConnection) Status() int {
	return http.StatusForbidden
}

func (e *InsecureConnection) Message() string {
	return "Secure Connection Required"
}

type UnsupportedEndpoint struct {
	request *http.Request
}

func (e *UnsupportedEndpoint) Code() string {
	return "unsupported_endpoint"
}

func (e *UnsupportedEndpoint) Error() string {
	return fmt.Sprintf("%s - Request: %s", e.Code(), formatRequest(e.request))
}

func (e *UnsupportedEndpoint) Status() int {
	return http.StatusNotFound
}

func (e *UnsupportedEndpoint) Message() string {
	return http.StatusText(e.Status())
}

type DeprecatedApiVersion struct {
	version int
	request *http.Request
}

func (e *DeprecatedApiVersion) Code() string {
	return "deprecated_api_version"
}

func (e *DeprecatedApiVersion) Error() string {
	return fmt.Sprintf("%s - Version: %d; Request: %s", e.Code(), e.version, formatRequest(e.request))
}

func (e *DeprecatedApiVersion) Status() int {
	return http.StatusNotAcceptable
}

func (e *DeprecatedApiVersion) Message() string {
	return fmt.Sprintf("The api version you are using (%d) has been deprecated. Please use version %d", e.version, ApiVersion)
}

type ServerError struct {
	error
	request *http.Request
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
