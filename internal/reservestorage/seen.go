package reservestorage

import (
	"encoding/binary"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

func keySeenTx(id string) []byte { return []byte("seen:tx:" + id) }
func keySeenBlk(h string) []byte { return []byte("seen:blk:" + h) }

func MarkSeenTx(db *leveldb.DB, id string) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(time.Now().Unix()))
	_ = db.Put(keySeenTx(id), b, nil)
}
func HasSeenTx(db *leveldb.DB, id string) bool { _, err := db.Get(keySeenTx(id), nil); return err == nil }

func MarkSeenBlk(db *leveldb.DB, h string) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(time.Now().Unix()))
	_ = db.Put(keySeenBlk(h), b, nil)
}
func HasSeenBlk(db *leveldb.DB, h string) bool { _, err := db.Get(keySeenBlk(h), nil); return err == nil }
