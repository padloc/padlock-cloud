package main

import "reflect"
import "errors"
import "encoding/json"
import "path/filepath"
import "github.com/MaKleSoft/padlock-cloud/Godeps/_workspace/src/github.com/syndtr/goleveldb/leveldb"

var (
	ErrStorableTypeNotSupported = errors.New("padlock: storable type not supported")
	ErrNotFound                 = errors.New("padlock: not found")
	ErrStorageClosed            = errors.New("padlock: storage closed")
)

type Storable interface {
	Key() []byte
	Serialize() ([]byte, error)
	Deserialize([]byte) error
}

type Storage interface {
	Open() error
	Close() error
	Get(Storable) error
	Put(Storable) error
	Delete(Storable) error
}

var StorableTypes = map[reflect.Type]string{
	reflect.TypeOf((*Data)(nil)).Elem():         "data",
	reflect.TypeOf((*Account)(nil)).Elem():      "auth",
	reflect.TypeOf((*AuthRequest)(nil)).Elem():  "act",
	reflect.TypeOf((*ResetRequest)(nil)).Elem(): "del",
}

type LevelDBStorage struct {
	Path   string
	stores map[reflect.Type]*leveldb.DB
}

func (s *LevelDBStorage) Open() error {
	s.stores = make(map[reflect.Type]*leveldb.DB)

	for t, loc := range StorableTypes {
		db, err := leveldb.OpenFile(filepath.Join(s.Path, loc), nil)
		if err != nil {
			return err
		}
		s.stores[t] = db
	}

	return nil
}

func (s *LevelDBStorage) Close() error {
	var err error

	for _, db := range s.stores {
		err = db.Close()
		if err != nil {
			return err
		}
	}

	s.stores = nil

	return nil
}

func (s *LevelDBStorage) getDB(t Storable) (*leveldb.DB, error) {
	db := s.stores[reflect.TypeOf(t).Elem()]

	if db == nil {
		return nil, ErrStorableTypeNotSupported
	}

	return db, nil
}

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
