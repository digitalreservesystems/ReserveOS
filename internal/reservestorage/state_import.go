package reservestorage

import (
	"encoding/binary"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func WipeStatePrefixes(db *leveldb.DB) error {
	for _, pref := range [][]byte{[]byte("bal:"), []byte("nonce:")} {
		it := db.NewIterator(util.BytesPrefix(pref), nil)
		for it.Next() { _ = db.Delete(it.Key(), nil) }
		it.Release()
		if err := it.Error(); err != nil { return err }
	}
	return nil
}

func ImportStateMaps(db *leveldb.DB, bal map[string]int64, non map[string]uint64) error {
	for k, v := range bal {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(v))
		_ = db.Put([]byte("bal:"+k), b, nil)
	}
	for k, v := range non {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, v)
		_ = db.Put([]byte("nonce:"+k), b, nil)
	}
	return nil
}
