package reservestorage

import (
	"encoding/json"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
)

func keyHeaderByHash(hashHex string) []byte { return []byte("hdr:hash:" + hashHex) }

func PutHeader(db *leveldb.DB, hashHex string, header any) error {
	b, err := json.Marshal(header)
	if err != nil { return err }
	return db.Put(keyHeaderByHash(hashHex), b, nil)
}

func GetHeaderByHash(db *leveldb.DB, hashHex string, out any) (bool, error) {
	b, err := db.Get(keyHeaderByHash(hashHex), nil)
	if err == nil {
		return true, json.Unmarshal(b, out)
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return false, nil
	}
	return false, err
}
