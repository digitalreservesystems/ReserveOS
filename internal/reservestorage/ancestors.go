package reservestorage

import (
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
	"reserveos/core/chain"
)

// GetAncestorHashAtHeight walks prev pointers from a tip hash down to targetHeight (inclusive).
// Returns ancestor hash string if reachable.
func GetAncestorHashAtHeight(db *leveldb.DB, tipHash string, targetHeight uint64) (string, bool, error) {
	cur := tipHash
	for {
		var b chain.Block
		ok, _, err := GetBlockByHash(db, cur, &b)
		if err != nil { return "", false, err }
		if !ok { return "", false, nil }
		if b.Header.Height == targetHeight { return b.Hash().String(), true, nil }
		if b.Header.Height < targetHeight { return "", false, nil }
		if b.Header.Height == 0 { return "", false, nil }
		cur = b.Header.PrevHash
	}
}

var _ = errors.Is
