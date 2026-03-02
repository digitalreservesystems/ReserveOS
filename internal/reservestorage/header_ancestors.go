package reservestorage

import (
	"github.com/syndtr/goleveldb/leveldb"
)

// GetHeaderAncestorHashAtHeight walks header prev pointers from a tip header hash down to targetHeight.
func GetHeaderAncestorHashAtHeight(db *leveldb.DB, tipHash string, targetHeight uint64) (string, bool, error) {
	cur := tipHash
	for {
		hdr, ok, err := GetHeaderByHash(db, cur)
		if err != nil { return "", false, err }
		if !ok { return "", false, nil }
		if hdr.Height == targetHeight { return cur, true, nil }
		if hdr.Height < targetHeight { return "", false, nil }
		if hdr.Height == 0 { return "", false, nil }
		cur = hdr.PrevHash.String()
	}
}
