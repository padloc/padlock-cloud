package padlockcloud

import "encoding/base64"
import "encoding/hex"
import "crypto/rand"
import "os"
import "path/filepath"

const tokenPattern = `[a-zA-Z0-9\-_]{22}`

var gopath = os.Getenv("GOPATH")
var DefaultAssetsPath = filepath.Join(gopath, "src/github.com/padlock/padlock-cloud/assets")

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func randomBase64(nBytes int) (string, error) {
	b, err := randomBytes(nBytes)

	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomHex(nBytes int) (string, error) {
	b, err := randomBytes(nBytes)

	if err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}

func token() (string, error) {
	return randomBase64(16)
}
