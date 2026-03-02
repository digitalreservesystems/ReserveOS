package reservestorage

import (
	"encoding/binary"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
)

var keyFrontierH = []byte("sync:frontier_height")
var keyFrontierHash = []byte("sync:frontier_hash")

func GetHeaderFrontier(db *leveldb.DB) (uint64, string, bool, error) {
	hb, err := db.Get(keyFrontierH, nil)
	if err == nil && len(hb) == 8 {
		h := binary.BigEndian.Uint64(hb)
		hashb, err2 := db.Get(keyFrontierHash, nil)
		hash := ""
		if err2 == nil { hash = string(hashb) }
		return h, hash, true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) { return 0, "", false, nil }
	return 0, "", false, err
}

func SetHeaderFrontier(db *leveldb.DB, h uint64, hash string) error {
	hb := make([]byte, 8)
	binary.BigEndian.PutUint64(hb, h)
	if err := db.Put(keyFrontierH, hb, nil); err != nil { return err }
	if hash != "" {
		_ = db.Put(keyFrontierHash, []byte(hash), nil)
	}
	return nil
}
