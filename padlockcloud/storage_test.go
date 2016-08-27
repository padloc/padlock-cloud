package padlockcloud

import "testing"
import "io/ioutil"
import "os"
import "fmt"

type testStrbl string

func (m *testStrbl) Key() []byte {
	return []byte("somekey")
}

func (m *testStrbl) Serialize() ([]byte, error) {
	return []byte(*m), nil
}

func (m *testStrbl) Deserialize(data []byte) error {
	*m = testStrbl(string(data))
	return nil
}

func TestLevelDBStorage(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	storage := &LevelDBStorage{
		Config: &LevelDBConfig{
			Path: dir,
		},
	}

	var storable testStrbl = "All work and no joy makes jack a dull boy"

	// Storage.Open() has not been called yet so we should get the appropriate error
	if err := storage.Get(&storable); err != ErrStorageClosed {
		t.Fatalf("Should return error for closed storage, got %v", err)
	}

	// Let's open the storage very quickly even though we haven't registered any types yet so we can
	// check for the ErrStorableTypeNotSupported error
	storage.Open()
	// Storage.Open() has not been called yet so we should get the appropriate error
	if err := storage.Get(&storable); err != ErrStorableTypeNotSupported {
		t.Fatalf("Should return error for unregistered storable type, got %v", err)
	}
	storage.Close()

	// No register the storable type
	AddStorable(&storable, "mystrbl")

	// Open storage now
	storage.Open()
	defer storage.Close()

	// Haven't written anything to the storage yet, so `Storage.List()` should give us an empty slice
	list, err := storage.List(&storable)
	if err != nil {
		t.Fatalf("Should return no error, got %v", err)
	}
	listStr := fmt.Sprintf("%s", list)
	if listStr != "[]" {
		t.Fatalf("Expected '%s', got '%s'", "[]", listStr)
	}

	// Still haven't written anything to storage, so trying to get a specific instace should give us
	// ErrNotFound
	if err := storage.Get(&storable); err != ErrNotFound {
		t.Fatalf("Should get error not found, got %v", err)
	}

	// Finally writing something. This should work without any incidents
	if err := storage.Put(&storable); err != nil {
		t.Fatalf("Should return no error, got %v", err)
	}

	// Initialize new storable and try to load data into it. This should work fine now and give us the
	// correct data
	var storable2 testStrbl
	if err := storage.Get(&storable2); err != nil {
		t.Fatalf("Should return no error, got %v", err)
	}
	if storable2 != storable {
		t.Fatalf("Expected '%s', got '%s'", storable, storable2)
	}

	// Now that we have a database entry we should have something in the list as well
	list, err = storage.List(&storable)
	if err != nil {
		t.Fatalf("Should return no error, got %v", err)
	}
	listStr = fmt.Sprintf("%s", list)
	if listStr != "[somekey]" {
		t.Fatalf("Expected '%s', got '%s'", "[somekey]", listStr)
	}

	// Lets delete our one entry again. This should work without any incidents
	if err := storage.Delete(&storable); err != nil {
		t.Fatalf("Should return no error, got %v", err)
	}

	// Now that we've deleted the entry, we should get ErrNotFound again when trying to load data for it
	if err := storage.Get(&storable); err != ErrNotFound {
		t.Fatalf("Should get error not found, got %v", err)
	}

}
