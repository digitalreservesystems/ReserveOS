package state

import (
	"encoding/binary"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
)

func keyBal(id string) []byte { return []byte("bal:" + id) }

func GetBalance(db *leveldb.DB, id string) (int64, bool, error) {
	b, err := db.Get(keyBal(id), nil)
	if err == nil {
		v := int64(binary.BigEndian.Uint64(b))
		return v, true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return 0, false, nil
	}
	return 0, false, err
}

func AddBalance(db *leveldb.DB, id string, delta int64) (int64, error) {
	cur, _, err := GetBalance(db, id)
	if err != nil { return 0, err }
	next := cur + delta
	if next < 0 { next = 0 } // v1 safety clamp
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(next))
	if err := db.Put(keyBal(id), b, nil); err != nil { return 0, err }
	return next, nil
}
