package main

import "net/http"
import "github.com/rs/cors"

func Cors(handler http.Handler) http.Handler {
	return cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"HEAD", "GET", "POST", "PUT", "DELETE"},
		AllowedHeaders: []string{"Authorization", "Accept", "Content-Type", "Require-Subscription"},
	}).Handler(handler)
}
