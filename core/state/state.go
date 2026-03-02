package state

import (
	"encoding/binary"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func keyBal(id string) []byte   { return []byte("bal:" + id) }
func keyNonce(id string) []byte { return []byte("nonce:" + id) }

func GetBalance(db *leveldb.DB, id string) (int64, bool, error) {
	b, err := db.Get(keyBal(id), nil)
	if err == nil {
		v := int64(binary.BigEndian.Uint64(b))
		return v, true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return 0, false, nil
	}
	return 0, false, err
}

func SetBalance(db *leveldb.DB, id string, value int64) error {
	if value < 0 { value = 0 }
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(value))
	return db.Put(keyBal(id), b, nil)
}

func AddBalance(db *leveldb.DB, id string, delta int64) (int64, error) {
	cur, _, err := GetBalance(db, id)
	if err != nil { return 0, err }
	next := cur + delta
	if next < 0 { return 0, errors.New("insufficient funds") }
	if err := SetBalance(db, id, next); err != nil { return 0, err }
	return next, nil
}

func GetNonce(db *leveldb.DB, id string) (uint64, bool, error) {
	b, err := db.Get(keyNonce(id), nil)
	if err == nil {
		return binary.BigEndian.Uint64(b), true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return 0, false, nil
	}
	return 0, false, err
}

func SetNonce(db *leveldb.DB, id string, nonce uint64) error {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, nonce)
	return db.Put(keyNonce(id), b, nil)
}

func ResetState(db *leveldb.DB) error {
	// delete all bal:* and nonce:* keys
	it := db.NewIterator(util.BytesPrefix([]byte("bal:")), nil)
	batch := new(leveldb.Batch)
	for it.Next() {
		batch.Delete(append([]byte{}, it.Key()...))
	}
	it.Release()
	it2 := db.NewIterator(util.BytesPrefix([]byte("nonce:")), nil)
	for it2.Next() {
		batch.Delete(append([]byte{}, it2.Key()...))
	}
	it2.Release()
	return db.Write(batch, nil)
}
