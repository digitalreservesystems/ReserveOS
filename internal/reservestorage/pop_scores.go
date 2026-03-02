package reservestorage

import (
	"encoding/binary"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func keyScore(id string) []byte { return []byte("pop:score:" + id) }

func AddScore(db *leveldb.DB, id string, delta uint64) error {
	cur, _ := GetScore(db, id)
	next := cur + delta
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, next)
	return db.Put(keyScore(id), b, nil)
}

func GetScore(db *leveldb.DB, id string) (uint64, bool) {
	b, err := db.Get(keyScore(id), nil)
	if err == nil {
		return binary.BigEndian.Uint64(b), true
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return 0, false
	}
	return 0, false
}

func ResetScores(db *leveldb.DB) error {
	it := db.NewIterator(util.BytesPrefix([]byte("pop:score:")), nil)
	batch := new(leveldb.Batch)
	for it.Next() {
		batch.Delete(append([]byte{}, it.Key()...))
	}
	it.Release()
	return db.Write(batch, nil)
}
