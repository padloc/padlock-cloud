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
		templates.ActivateAuthTokenSuccess == nil ||
		templates.DeprecatedVersionEmail == nil ||
		templates.ErrorPage == nil ||
		templates.LoginPage == nil ||
		templates.Dashboard == nil ||
		templates.DeleteStore == nil {
		t.Fatal("All templates should be initialized and not nil")
	}
}
