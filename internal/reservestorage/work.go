package reservestorage

import (
	"encoding/binary"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
)

func keyWork(hashHex string) []byte { return []byte("work:" + hashHex) }

func PutWork(db *leveldb.DB, hashHex string, work uint64) error {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, work)
	return db.Put(keyWork(hashHex), b, nil)
}

func GetWork(db *leveldb.DB, hashHex string) (uint64, bool, error) {
	b, err := db.Get(keyWork(hashHex), nil)
	if err == nil {
		return binary.BigEndian.Uint64(b), true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return 0, false, nil
	}
	return 0, false, err
}
