package padlockcloud

import "log"
import "net/http"
import "gopkg.in/throttled/throttled.v2"
import "gopkg.in/throttled/throttled.v2/store/memstore"

type RateQuota throttled.RateQuota

var PerSec = throttled.PerSec
var PerMin = throttled.PerMin

type Route struct {
	Url    string
	Method string
}

// Limits the rate of a given handler to a certain number of requests per minute
func RateLimit(handler http.Handler, quotas map[Route]RateQuota, deniedHandler http.Handler) http.Handler {
	store, err := memstore.New(65536)
	if err != nil {
		log.Fatal(err)
	}

	rateLimiters := make(map[Route]http.Handler)

	for route, quota := range quotas {
		rateLimiter, err := throttled.NewGCRARateLimiter(store, throttled.RateQuota(quota))
		if err != nil {
			log.Fatal(err)
		}
		rateLimiters[route] = (&throttled.HTTPRateLimiter{
			RateLimiter: rateLimiter,
			VaryBy: &throttled.VaryBy{
				RemoteAddr: true,
				Path:       true,
				Method:     true,
				// For apps running behind a reverse proxy, the RemoteAddr field is likely to not contain the
				// actual IP, so check for the X-Real-IP header also, which needs to be set by the reverse proxy
				Headers: []string{"X-Real-IP"},
			},
			DeniedHandler: deniedHandler,
		}).RateLimit(handler)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := Route{r.Method, r.URL.Path}
		rateLimiter := rateLimiters[route]

		if rateLimiter != nil {
			rateLimiter.ServeHTTP(w, r)
		} else {
			handler.ServeHTTP(w, r)
		}
	})
}
