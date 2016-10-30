package padlockcloud

import "net/http"
import "github.com/rs/cors"

func Cors(handler http.Handler) http.Handler {
	return cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"HEAD", "GET", "POST", "PUT", "DELETE"},
		AllowedHeaders: []string{"Authorization", "Accept", "Content-Type", "X-Client-Version"},
		ExposedHeaders: []string{"X-Sub-Required", "X-Sub-Status", "X-Sub-Trial-End"},
	}).Handler(handler)
}
