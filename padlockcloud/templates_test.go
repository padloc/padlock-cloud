package padlockcloud

import "os"
import "io/ioutil"
import "path/filepath"
import "testing"

func TestLoadTemplates(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	if _, err := LoadTemplates(dir); err == nil {
		t.Fatal("Trying to load templates from empty or unexisting directory should return an error")
	}

	templates, err := LoadTemplates(filepath.Join(DefaultAssetsPath, "templates"))
	if err != nil {
		t.Fatalf("Loading templates from default dir should work without errors, got %v", err)
	}

	if templates.ActivateAuthTokenEmail == nil ||
		templates.DeleteStoreEmail == nil ||
		templates.ActivateAuthTokenSuccess == nil ||
		templates.DeleteStoreSuccess == nil ||
		templates.DeprecatedVersionEmail == nil {
		t.Fatal("All templates should be initialized and not nil")
	}
}
