package main

import "encoding/base64"
import "crypto/rand"

const tokenPattern = `[a-zA-Z0-9\-_]{22}`

func randomBase64(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func token() (string, error) {
	return randomBase64(16)
}
