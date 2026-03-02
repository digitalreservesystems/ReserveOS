package reservestorage

import (
	"encoding/binary"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
)

func keyBlockFees(height uint64) []byte { return []byte(fmt.Sprintf("fees:block:%d", height)) }

func PutBlockFeeSum(db *leveldb.DB, height uint64, sum int64) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(sum))
	_ = db.Put(keyBlockFees(height), b, nil)
}

func GetBlockFeeSum(db *leveldb.DB, height uint64) (int64, bool) {
	b, err := db.Get(keyBlockFees(height), nil)
	if err != nil || len(b)!=8 { return 0, false }
	u := binary.BigEndian.Uint64(b)
	return int64(u), true
}
