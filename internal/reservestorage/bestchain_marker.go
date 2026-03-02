package reservestorage

import (
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
)

var keyBestChainTip = []byte("bestchain:tip_hash")

func SetBestChainTip(db *leveldb.DB, hash string) error {
	return db.Put(keyBestChainTip, []byte(hash), nil)
}

func GetBestChainTip(db *leveldb.DB) (string, bool, error) {
	b, err := db.Get(keyBestChainTip, nil)
	if err == nil { return string(b), true, nil }
	if errors.Is(err, leveldb.ErrNotFound) { return "", false, nil }
	return "", false, err
}
