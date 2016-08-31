package padlockcloud

import "fmt"
import "net/http"

type ErrorResponse interface {
	error
	Code() string
	Status() int
}

type BadRequest struct {
	request *http.Request
}

func (e *BadRequest) Code() string {
	return "bad_request"
}

func (e *BadRequest) Error() string {
	return "Bad request"
}

func (e *BadRequest) Status() int {
	return http.StatusBadRequest
}

type InvalidToken struct {
	token   string
	request *http.Request
}

func (e *InvalidToken) Code() string {
	return "invalid_token"
}

func (e *InvalidToken) Error() string {
	return fmt.Sprintf("Invalid token: %s", e.token)
}

func (e *InvalidToken) Status() int {
	return http.StatusBadRequest
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
	return fmt.Sprintf("Unauthorized - Url: '%s', Email: '%s', token: '%s'", e.request.URL, e.email, e.token)
}

func (e *Unauthorized) Status() int {
	return http.StatusUnauthorized
}

type MethodNotAllowed struct {
	request *http.Request
}

func (e *MethodNotAllowed) Code() string {
	return "method_not_allowed"
}

func (e *MethodNotAllowed) Error() string {
	return fmt.Sprintf("Method not allowed - %s %s", e.request.Method, e.request.URL)
}

func (e *MethodNotAllowed) Status() int {
	return http.StatusMethodNotAllowed
}

type InsecureConnection struct {
	request *http.Request
}

func (e *InsecureConnection) Code() string {
	return "insecure_connection"
}

func (e *InsecureConnection) Error() string {
	return fmt.Sprintf("Insecure connection - %s %s", e.request.Method, e.request.URL)
}

func (e *InsecureConnection) Status() int {
	return http.StatusForbidden
}

type UnsupportedEndpoint struct {
	request *http.Request
}

func (e *UnsupportedEndpoint) Code() string {
	return "unsupported_endpoint"
}

func (e *UnsupportedEndpoint) Error() string {
	return fmt.Sprintf("Unsupported endpoint - %s %s", e.request.Method, e.request.URL)
}

func (e *UnsupportedEndpoint) Status() int {
	return http.StatusNotFound
}

type DeprecatedApiVersion struct {
	version int
	request *http.Request
}

func (e *DeprecatedApiVersion) Code() string {
	return "deprecated_api_version"
}

func (e *DeprecatedApiVersion) Error() string {
	return fmt.Sprintf("Deprecated version - %d", e.version)
}

func (e *DeprecatedApiVersion) Status() int {
	return http.StatusNotAcceptable
}

type InternalServerError struct {
	error
	request *http.Request
}

func (e *InternalServerError) Code() string {
	return "internal_server_error"
}

func (e *InternalServerError) Status() int {
	return http.StatusInternalServerError
}

func WriteErrorResponse(e ErrorResponse, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status())
	w.Write([]byte(fmt.Sprintf("{\"error\": \"%s\"}", e.Code())))
}
