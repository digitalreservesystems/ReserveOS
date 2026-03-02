package reservestorage

import (
	"encoding/binary"
	"encoding/json"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
)

func keyBlockByHash(hashHex string) []byte { return []byte("blk:hash:" + hashHex) }

func keyHashByHeight(h uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, h)
	return append([]byte("blk:height:"), b...)
}

func PutBlock(db *leveldb.DB, hashHex string, height uint64, block any) error {
	b, err := json.Marshal(block)
	if err != nil {
		return err
	}
	batch := new(leveldb.Batch)
	batch.Put(keyBlockByHash(hashHex), b)
	batch.Put(keyHashByHeight(height), []byte(hashHex))
	return db.Write(batch, nil)
}

func GetHashByHeight(db *leveldb.DB, height uint64) (string, bool, error) {
	b, err := db.Get(keyHashByHeight(height), nil)
	if err == nil {
		return string(b), true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return "", false, nil
	}
	return "", false, err
}

func GetBlockByHash(db *leveldb.DB, hashHex string, out any) (bool, error) {
	b, err := db.Get(keyBlockByHash(hashHex), nil)
	if err == nil {
		return true, json.Unmarshal(b, out)
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return false, nil
	}
	return false, err
}


// PutHashByHeightOverwrite updates the main-chain height index to point to hashHex.
// Fork blocks remain addressable by hash; this only changes the selected main chain mapping.
func PutHashByHeightOverwrite(db *leveldb.DB, height uint64, hashHex string) error {
	return db.Put(keyHashByHeight(height), []byte(hashHex), nil)
}
