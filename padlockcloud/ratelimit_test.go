package padlockcloud

import "net/http"
import "net/http/httptest"
import "testing"
import "time"

func TestRateLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rl := RateLimit(handler, map[Route]RateQuota{
		Route{"GET", "/test/"}: RateQuota{PerSec(1), 0},
	}, nil)

	testServer := httptest.NewServer(rl)

	var res *http.Response
	var err error

	// First request to rate-limited route should go through fine
	if res, err = http.Get(testServer.URL + "/test/"); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected OK as status, got %s", res.Status)
	}

	// Second request should be rate limited
	if res, err = http.Get(testServer.URL + "/test/"); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("Expected too many requests status code, got %s", res.Status)
	}

	// Check if retry-after header was set correctly
	retryAfter := res.Header.Get("Retry-After")
	if retryAfter != "1" {
		t.Fatalf("Expected Retry-After header to be 1, found %s", retryAfter)
	}

	// Lets wait a second and try again
	time.Sleep(time.Second)

	// Endpoint should be available again
	if res, err = http.Get(testServer.URL + "/test/"); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected OK as status, got %s", res.Status)
	}

	// Non rate-limited routes can be called in quick succession
	if res, err = http.Get(testServer.URL + "/blah/"); err != nil {
		t.Fatal(err)
	}
	if res, err = http.Get(testServer.URL + "/blah/"); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected OK as status, got %s", res.Status)
	}
}
