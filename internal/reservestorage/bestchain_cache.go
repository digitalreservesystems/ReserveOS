package reservestorage

import (
	"errors"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
)

func keyBestAtHeight(h uint64) []byte { return []byte(fmt.Sprintf("besth:%d", h)) }

func PutBestHashAtHeight(db *leveldb.DB, h uint64, hash string) error {
	return db.Put(keyBestAtHeight(h), []byte(hash), nil)
}

func GetBestHashAtHeight(db *leveldb.DB, h uint64) (string, bool, error) {
	b, err := db.Get(keyBestAtHeight(h), nil)
	if err == nil { return string(b), true, nil }
	if errors.Is(err, leveldb.ErrNotFound) { return "", false, nil }
	return "", false, err
}
