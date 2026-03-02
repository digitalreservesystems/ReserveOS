package reservestorage

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
)

var (
	keyFinalizedHeight = []byte("meta:finalized_height")
	keyFinalizedHash   = []byte("meta:finalized_hash")
)

func keyCheckpoint(height uint64) []byte { return []byte(fmt.Sprintf("finality:checkpoint:%d", height)) }

func GetFinalized(db *leveldb.DB) (height uint64, hashHex string, ok bool, err error) {
	hb, err := db.Get(keyFinalizedHeight, nil)
	if err == nil {
		if err := json.Unmarshal(hb, &height); err != nil {
			return 0, "", false, err
		}
		hh, err := db.Get(keyFinalizedHash, nil)
		if err != nil {
			return 0, "", false, err
		}
		return height, string(hh), true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return 0, "", false, nil
	}
	return 0, "", false, err
}

func PutFinalized(db *leveldb.DB, height uint64, hashHex string) error {
	hb, _ := json.Marshal(height)
	if err := db.Put(keyFinalizedHeight, hb, nil); err != nil {
		return err
	}
	return db.Put(keyFinalizedHash, []byte(hashHex), nil)
}

func PutCheckpoint(db *leveldb.DB, height uint64, checkpoint any) error {
	b, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	return db.Put(keyCheckpoint(height), b, nil)
}
