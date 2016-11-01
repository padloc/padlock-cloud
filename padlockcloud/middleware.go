package padlockcloud

import "net/http"
import "errors"
import "fmt"
import "strings"
import "github.com/gorilla/csrf"

var CSRFTemplateTag = csrf.TemplateTag
var CSRFTemplateField = csrf.TemplateField

type MiddleWare interface {
	Wrap(Handler) Handler
}

type CheckEndpointVersion struct {
	*Server
	Version int
}

func (m *CheckEndpointVersion) Wrap(h Handler) Handler {
	return HandlerFunc(func(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
		// Contains deprecated 'ApiKey email:token' authentication scheme
		depAuth := strings.Contains(r.Header.Get("Authorization"), "ApiKey")

		version := versionFromRequest(r)

		if depAuth || m.Version != 0 && version != m.Version {
			m.SendDeprecatedVersionEmail(r)
			return &UnsupportedApiVersion{version, m.Version}
		}

		return h.Handle(w, r, auth)
	})
}

type Authenticate struct {
	*Server
	Type string
}

func (m *Authenticate) Wrap(h Handler) Handler {
	return HandlerFunc(func(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
		// Get auth token from request
		auth, err := m.Authenticate(r)

		// Endpoint requires authentation but no auth token could be aquired
		if m.Type != "" && err != nil {
			// If this endpoint requires web authentication, simply redirect to login page
			if m.Type == "web" {
				http.Redirect(w, r, "/login/", http.StatusFound)
				return nil
			}

			return err
		}

		// Make sure auth token has the right type
		if m.Type != "" && auth.Type != m.Type {
			return &InvalidAuthToken{auth.Email, auth.Token}
		}

		return h.Handle(w, r, auth)
	})
}

type CSRF struct {
	*Server
}

func (m *CSRF) Wrap(h Handler) Handler {
	return HandlerFunc(func(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
		if auth != nil && auth.Type == "web" {
			// Wrap the handler function in a http.Handler; Capture error in `e` variable for
			// later use. We need to do this because the csrf middleware only works with a http.Handler
			var err error
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				err = h.Handle(w, r, auth)
			})

			handler = csrf.Protect(
				m.secret,
				csrf.Path("/"),
				csrf.Secure(m.Secure),
				csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					m.HandleError(&InvalidCsrfToken{csrf.FailureReason(r)}, w, r)
				})),
			)(handler)

			handler.ServeHTTP(w, r)

			return err
		} else {
			return h.Handle(w, r, auth)
		}
	})
}

type HandleError struct {
	*Server
}

func (m *HandleError) Wrap(h Handler) Handler {
	return HandlerFunc(func(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
		err := h.Handle(w, r, auth)

		if err != nil {
			m.HandleError(err, w, r)
		}

		return err
	})
}

type CheckMethod struct {
	Allowed map[string]Handler
}

func (m *CheckMethod) Wrap(h Handler) Handler {
	return HandlerFunc(func(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
		if m.Allowed[r.Method] == nil {
			return &MethodNotAllowed{r.Method}
		}

		return h.Handle(w, r, auth)
	})
}

type HandlePanic struct {
}

func (m *HandlePanic) Wrap(h Handler) Handler {
	return HandlerFunc(func(w http.ResponseWriter, r *http.Request, a *AuthToken) error {
		var err error

		func() {
			defer func() {
				if e := recover(); e != nil {
					var ok bool
					err, ok = e.(error)
					if !ok {
						err = errors.New(fmt.Sprintf("%v", e))
					}
				}
			}()

			err = h.Handle(w, r, a)
		}()

		return err
	})
}
