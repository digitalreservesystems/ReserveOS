package reservestorage

import (
	"encoding/binary"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
)

var keyFinalized = []byte("finality:finalized_height")

func GetFinalizedHeight(db *leveldb.DB) (uint64, bool, error) {
	b, err := db.Get(keyFinalized, nil)
	if err == nil {
		if len(b) != 8 { return 0, false, nil }
		return binary.BigEndian.Uint64(b), true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) { return 0, false, nil }
	return 0, false, err
}

func SetFinalizedHeight(db *leveldb.DB, h uint64) error {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, h)
	return db.Put(keyFinalized, b, nil)
}
