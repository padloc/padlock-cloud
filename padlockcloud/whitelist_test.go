package padlockcloud

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

const (
	whitelistTestEmail  string = "test@test.com"
	whitelistTestEmail2 string = "test2@test.com"
)

func TestLoadWhitelist(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	tmpFile := filepath.Join(dir, "tmpFile")
	if err := writeToFile(tmpFile); err != nil {
		t.Errorf("Error creating tmpfile: %s", err.Error())
	}

	w, err := NewWhitelist(tmpFile)
	if err != nil {
		t.Errorf("Error reading whitelist: %s", err.Error())
	}
	if w == nil {
		t.Fatalf("Whitelist should not be nil")
	}

	if !w.IsWhitelisted(whitelistTestEmail) {
		t.Errorf(fmt.Sprintf("%s should be whitelisted is not", whitelistTestEmail))
	}

	if !w.IsWhitelisted(whitelistTestEmail2) {
		t.Errorf(fmt.Sprintf("%s should be whitelisted is not", whitelistTestEmail2))
	}

	notWhitelisted := "not@whitelist.com"
	if w.IsWhitelisted(notWhitelisted) {
		t.Errorf(fmt.Sprintf("%s should not be whitelisted but is", notWhitelisted))
	}
}

func writeToFile(path string) error {
	d1 := []byte(whitelistTestEmail + "\n" + whitelistTestEmail2 + "\n")
	return ioutil.WriteFile(path, d1, 0644)
}
