package padlockcloud

import "encoding/base64"
import "crypto/rand"
import "os"
import "path/filepath"

const tokenPattern = `[a-zA-Z0-9\-_]{22}`

var gopath = os.Getenv("GOPATH")
var DefaultAssetsPath = filepath.Join(gopath, "src/github.com/maklesoft/padlock-cloud/assets")

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
