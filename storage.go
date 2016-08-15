package main

import "reflect"
import "errors"
import "encoding/json"
import "path/filepath"
import "github.com/syndtr/goleveldb/leveldb"

// Error singletons
var (
	// A particular implementation of the Storable implementation is not supported
	ErrStorableTypeNotSupported = errors.New("padlock: storable type not supported")
	// An object was not found
	ErrNotFound = errors.New("padlock: not found")
	// A query was attempted on a closed storage
	ErrStorageClosed = errors.New("padlock: storage closed")
)

// Common interface for types that can be stored using the `Storage` interface.
type Storable interface {
	// This method is used for retrieving a key hat can be used to identify an object
	// The returned value should be unique and constant
	Key() []byte
	// Creates a string representation of an object. Data returned from this method should
	// be able to be fed into the `Deserialize` method to retrieve the original state
	Serialize() ([]byte, error)
	// Populates the fields from serialized data.
	Deserialize([]byte) error
}

// Common interface for storage implementations
type Storage interface {
	// Prepares the database for use
	Open() error
	// Closes the database and performs cleanup actions
	Close() error
	// Populates a given `Storable` object with data retrieved from the store
	Get(Storable) error
	// Updates the store with the data from a given `Storable` object
	Put(Storable) error
	// Removes a given `Storable` object from the store
	Delete(Storable) error
	// Lists all keys for a given `Storable` type
	List(Storable) ([]string, error)
}

// Map of supported `Storable` implementations along with identifier strings that can be used for
// internal store or file names
var StorableTypes = map[reflect.Type]string{
	reflect.TypeOf((*Store)(nil)).Elem():              "data",
	reflect.TypeOf((*Account)(nil)).Elem():            "auth",
	reflect.TypeOf((*AuthRequest)(nil)).Elem():        "act",
	reflect.TypeOf((*DeleteStoreRequest)(nil)).Elem(): "del",
}

func AddStorable(t interface{}, loc string) {
	StorableTypes[reflect.TypeOf(t).Elem()] = loc
}

type LevelDBConfig struct {
	// Path to directory on disc where database files should be stored
	Path string
}

// LevelDB implementation of the `Storage` interface
type LevelDBStorage struct {
	LevelDBConfig
	// Map of `leveldb.DB` instances associated with different `Storable` types
	stores map[reflect.Type]*leveldb.DB
}

// Implementation of the `Storage.Open` interface method
func (s *LevelDBStorage) Open() error {
	// Instantiate stores map
	s.stores = make(map[reflect.Type]*leveldb.DB)

	// Create `leveldb.DB` instance for each supported `Storable` type
	for t, loc := range StorableTypes {
		db, err := leveldb.OpenFile(filepath.Join(s.Path, loc), nil)
		if err != nil {
			return err
		}
		s.stores[t] = db
	}

	return nil
}

// Implementation of the `Storage.Close` interface method
func (s *LevelDBStorage) Close() error {
	var err error

	// Close all existing `leveldb.DB` instances
	for _, db := range s.stores {
		err = db.Close()
		if err != nil {
			return err
		}
	}

	s.stores = nil

	return nil
}

// Get `leveldb.DB` instance for a given type
func (s *LevelDBStorage) getDB(t Storable) (*leveldb.DB, error) {
	db := s.stores[reflect.TypeOf(t).Elem()]

	if db == nil {
		return nil, ErrStorableTypeNotSupported
	}

	return db, nil
}

// Implementation of the `Storage.Get` interface method
func (s *LevelDBStorage) Get(t Storable) error {
	if s.stores == nil {
		return ErrStorageClosed
	}

	if t == nil {
		return ErrStorableTypeNotSupported
	}

	db, err := s.getDB(t)
	if err != nil {
		return err
	}

	data, err := db.Get(t.Key(), nil)
	if err == leveldb.ErrNotFound {
		return ErrNotFound
	} else if err != nil {
		return err
	}

	return t.Deserialize(data)
}

// Implementation of the `Storage.Put` interface method
func (s *LevelDBStorage) Put(t Storable) error {
	if s.stores == nil {
		return ErrStorageClosed
	}

	if t == nil {
		return ErrStorableTypeNotSupported
	}

	db, err := s.getDB(t)
	if err != nil {
		return err
	}

	data, err := t.Serialize()
	if err != nil {
		return err
	}

	return db.Put(t.Key(), data, nil)
}

// Implementation of the `Storage.Delete` interface method
func (s *LevelDBStorage) Delete(t Storable) error {
	if s.stores == nil {
		return ErrStorageClosed
	}

	if t == nil {
		return ErrStorableTypeNotSupported
	}

	db, err := s.getDB(t)
	if err != nil {
		return err
	}

	return db.Delete(t.Key(), nil)
}

func (s *LevelDBStorage) List(t Storable) ([]string, error) {
	var keys []string

	db, err := s.getDB(t)
	if err != nil {
		return keys, err
	}

	iter := db.NewIterator(nil, nil)
	for iter.Next() {
		keys = append(keys, string(iter.Key()))
	}
	iter.Release()

	return keys, nil
}

// In-memory implemenation of the `Storage` interface Mainly used for testing
type MemoryStorage struct {
	store map[reflect.Type](map[string][]byte)
}

func (s *MemoryStorage) Open() error {
	s.store = make(map[reflect.Type](map[string][]byte))
	return nil
}

func (s *MemoryStorage) Close() error {
	return nil
}

func (s *MemoryStorage) Get(t Storable) error {
	if s.store == nil {
		return ErrStorageClosed
	}

	if t == nil {
		return ErrStorableTypeNotSupported
	}

	tm := s.store[reflect.TypeOf(t)]
	if tm == nil {
		return ErrNotFound
	}
	data := tm[string(t.Key())]
	if data == nil {
		return ErrNotFound
	}
	return json.Unmarshal(data, t)
}

func (s *MemoryStorage) Put(t Storable) error {
	if s.store == nil {
		return ErrStorageClosed
	}

	if t == nil {
		return ErrStorableTypeNotSupported
	}

	data, err := json.Marshal(t)
	if err != nil {
		return err
	}

	if s.store[reflect.TypeOf(t)] == nil {
		s.store[reflect.TypeOf(t)] = make(map[string][]byte)
	}
	s.store[reflect.TypeOf(t)][string(t.Key())] = data

	return nil
}

func (s *MemoryStorage) Delete(t Storable) error {
	if s.store == nil {
		return ErrStorageClosed
	}

	if t == nil {
		return ErrStorableTypeNotSupported
	}

	ts := s.store[reflect.TypeOf(t)]
	if ts != nil {
		delete(ts, string(t.Key()))
	}
	return nil
}

func (s *MemoryStorage) List(t Storable) ([]string, error) {
	var l []string

	if s.store == nil {
		return l, ErrStorageClosed
	}

	if t == nil {
		return l, ErrStorableTypeNotSupported
	}

	ts := s.store[reflect.TypeOf(t)]
	if ts == nil {
		return l, ErrStorableTypeNotSupported
	}

	for key, _ := range ts {
		l = append(l, key)
	}

	return l, nil
}
