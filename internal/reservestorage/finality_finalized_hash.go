package reservestorage

import (
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
)

var keyFinalizedHash = []byte("finality:finalized_hash")

func GetFinalizedHash(db *leveldb.DB) (string, bool, error) {
	b, err := db.Get(keyFinalizedHash, nil)
	if err == nil { return string(b), true, nil }
	if errors.Is(err, leveldb.ErrNotFound) { return "", false, nil }
	return "", false, err
}

func SetFinalizedHash(db *leveldb.DB, hash string) error {
	return db.Put(keyFinalizedHash, []byte(hash), nil)
}
