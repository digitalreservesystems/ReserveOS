package reservestorage

import (
	"encoding/binary"
	"encoding/json"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
)

var (
	keyGenesis    = []byte("meta:genesis")
	keyTipHeight  = []byte("meta:tip_height")
	keyTipBlockID = []byte("meta:tip_block_id")
	keyTipWork    = []byte("meta:tip_work")
)

func HasGenesis(db *leveldb.DB) (bool, error) {
	_, err := db.Get(keyGenesis, nil)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return false, nil
	}
	return false, err
}

func PutGenesis(db *leveldb.DB, genesis any) error {
	b, err := json.Marshal(genesis)
	if err != nil {
		return err
	}
	return db.Put(keyGenesis, b, nil)
}

func GetGenesis(db *leveldb.DB, out any) (bool, error) {
	b, err := db.Get(keyGenesis, nil)
	if err == nil {
		return true, json.Unmarshal(b, out)
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return false, nil
	}
	return false, err
}

func PutTip(db *leveldb.DB, height uint64, blockID []byte) error {
	hb, _ := json.Marshal(height)
	if err := db.Put(keyTipHeight, hb, nil); err != nil {
		return err
	}
	return db.Put(keyTipBlockID, blockID, nil)
}

func GetTip(db *leveldb.DB) (height uint64, tipHashHex string, ok bool, err error) {
	hb, err := db.Get(keyTipHeight, nil)
	if err == nil {
		if err := json.Unmarshal(hb, &height); err != nil {
			return 0, "", false, err
		}
		th, err := db.Get(keyTipBlockID, nil)
		if err != nil {
			return 0, "", false, err
		}
		return height, string(th), true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return 0, "", false, nil
	}
	return 0, "", false, err
}


// SetTip rewrites the chain tip metadata to (height, hashHex). Used for safe reorg-from-finalized.
func SetTip(db *leveldb.DB, height uint64, hashHex string) error {
	return PutTip(db, height, []byte(hashHex))
}


func PutTipWork(db *leveldb.DB, work uint64) error {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, work)
	return db.Put(keyTipWork, b, nil)
}

func GetTipWork(db *leveldb.DB) (uint64, bool, error) {
	b, err := db.Get(keyTipWork, nil)
	if err == nil {
		return binary.BigEndian.Uint64(b), true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return 0, false, nil
	}
	return 0, false, err
}
