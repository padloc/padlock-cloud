package padlockcloud

import "reflect"
import "errors"
import "encoding/json"
import "path/filepath"
import "github.com/syndtr/goleveldb/leveldb"
import "github.com/syndtr/goleveldb/leveldb/iterator"

// Error singletons
var (
	// A particular implementation of the Storable implementation is not supported
	ErrUnregisteredStorable = errors.New("padlock: unregistered storable type")
	// An object was not found
	ErrNotFound = errors.New("padlock: not found")
	// A query was attempted on a closed storage
	ErrStorageClosed = errors.New("padlock: storage closed")
)

func typeFromStorable(t Storable) reflect.Type {
	return reflect.TypeOf(t).Elem()
}

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

type StorageIterator interface {
	Next() bool
	Get(Storable) error
	Release()
}

// Common interface for storage implementations
type Storage interface {
	// Prepares the database for use
	Open() error
	// Closes the database and performs cleanup actions
	Close() error
	// Returns readyness of the storage
	Ready() bool
	// Whether storage can store a certain storable
	CanStore(t Storable) bool
	// Populates a given `Storable` object with data retrieved from the store
	Get(Storable) error
	// Updates the store with the data from a given `Storable` object
	Put(Storable) error
	// Removes a given `Storable` object from the store
	Delete(Storable) error
	// Lists all keys for a given `Storable` type
	Iterator(Storable) (StorageIterator, error)
}

// Map of supported `Storable` implementations along with identifier strings that can be used for
// internal store or file names
var StorableTypes = map[reflect.Type]string{}

func RegisterStorable(t Storable, loc string) {
	StorableTypes[typeFromStorable(t)] = loc
}

type LevelDBIterator struct {
	iterator.Iterator
}

func (iter *LevelDBIterator) Get(t Storable) error {
	return t.Deserialize(iter.Value())
}

type LevelDBConfig struct {
	// Path to directory on disc where database files should be stored
	Path string `yaml:"path"`
}

// LevelDB implementation of the `Storage` interface
type LevelDBStorage struct {
	Config *LevelDBConfig
	// Map of `leveldb.DB` instances associated with different `Storable` types
	stores map[reflect.Type]*leveldb.DB
}

// Implementation of the `Storage.Open` interface method
func (s *LevelDBStorage) Open() error {
	// Instantiate stores map
	s.stores = make(map[reflect.Type]*leveldb.DB)

	// Create `leveldb.DB` instance for each supported `Storable` type
	for t, loc := range StorableTypes {
		db, err := leveldb.OpenFile(filepath.Join(s.Config.Path, loc), nil)
		if err != nil {
			return err
		}
		s.stores[t] = db
	}

	return nil
}

// Implementation of the `Storage.Close` interface method
func (s *LevelDBStorage) Close() error {
	// Close all existing `leveldb.DB` instances
	for _, db := range s.stores {
		if err := db.Close(); err != nil {
			return err
		}
	}

	s.stores = nil

	return nil
}

func (s *LevelDBStorage) Ready() bool {
	return s.stores != nil
}

func (s *LevelDBStorage) CanStore(t Storable) bool {
	_, err := s.getDB(t)
	return err == nil
}

// Get `leveldb.DB` instance for a given type
func (s *LevelDBStorage) getDB(t Storable) (*leveldb.DB, error) {
	db := s.stores[typeFromStorable(t)]

	if db == nil {
		return nil, ErrUnregisteredStorable
	}

	return db, nil
}

// Implementation of the `Storage.Get` interface method
func (s *LevelDBStorage) Get(t Storable) error {
	if s.stores == nil {
		return ErrStorageClosed
	}

	if t == nil {
		return ErrUnregisteredStorable
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
		return ErrUnregisteredStorable
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
		return ErrUnregisteredStorable
	}

	db, err := s.getDB(t)
	if err != nil {
		return err
	}

	return db.Delete(t.Key(), nil)
}

func (s *LevelDBStorage) Iterator(t Storable) (StorageIterator, error) {
	db, err := s.getDB(t)
	if err != nil {
		return nil, err
	}

	iter := db.NewIterator(nil, nil)
	return &LevelDBIterator{iter}, nil
}

type SliceIterator struct {
	s [][]byte
	i int
}

func (iter *SliceIterator) Next() bool {
	if iter.i < len(iter.s)-1 {
		iter.i = iter.i + 1
		return true
	}

	return false
}

func (iter *SliceIterator) Get(t Storable) error {
	return t.Deserialize(iter.s[iter.i])
}

func (iter *SliceIterator) Release() {
	iter.s = nil
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
		return ErrUnregisteredStorable
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
		return ErrUnregisteredStorable
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
		return ErrUnregisteredStorable
	}

	ts := s.store[reflect.TypeOf(t)]
	if ts != nil {
		delete(ts, string(t.Key()))
	}
	return nil
}

func (s *MemoryStorage) Ready() bool {
	return s.store != nil
}

func (s *MemoryStorage) CanStore(t Storable) bool {
	return true
}

func (s *MemoryStorage) Iterator(t Storable) (StorageIterator, error) {
	if s.store == nil {
		return nil, ErrStorageClosed
	}

	if t == nil {
		return nil, ErrUnregisteredStorable
	}

	ts := s.store[reflect.TypeOf(t)]
	if ts == nil {
		return nil, ErrUnregisteredStorable
	}

	var sl [][]byte
	for _, val := range ts {
		sl = append(sl, val)
	}

	return &SliceIterator{
		s: sl,
	}, nil
}
