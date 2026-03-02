package reservestorage

import (
	"encoding/binary"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
)

func keyCumWork(hashHex string) []byte { return []byte("cumwork:" + hashHex) }

func PutCumWork(db *leveldb.DB, hashHex string, cw uint64) error {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, cw)
	return db.Put(keyCumWork(hashHex), b, nil)
}

func GetCumWork(db *leveldb.DB, hashHex string) (uint64, bool, error) {
	b, err := db.Get(keyCumWork(hashHex), nil)
	if err == nil && len(b) == 8 {
		return binary.BigEndian.Uint64(b), true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) { return 0, false, nil }
	return 0, false, err
}
