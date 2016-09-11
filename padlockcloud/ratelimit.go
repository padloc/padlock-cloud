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

type VaryBy struct{}

func (v *VaryBy) Key(r *http.Request) string {
	return formatRequest(r)
}

// Limits the rate of a given handler to a certain number of requests per minute
func RateLimit(handler http.Handler, quotas map[Route]RateQuota, deniedHandler http.Handler) http.Handler {
	var varyBy *VaryBy

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
			RateLimiter:   rateLimiter,
			VaryBy:        varyBy,
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
