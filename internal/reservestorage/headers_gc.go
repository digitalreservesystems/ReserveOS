package reservestorage

import (
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"strings"
)

// DeleteHeadersBelowHeight removes header-by-hash entries for heights < hLimit.
// Best-effort, intended for pruning old forks once finalized.
func DeleteHeadersBelowHeight(db *leveldb.DB, hLimit uint64) {
	// delete height memberships first
	it := db.NewIterator(util.BytesPrefix([]byte("hdrh:")), nil)
	defer it.Release()
	for it.Next() {
		k := string(it.Key())
		// hdrh:<h>:<hash>
		parts := strings.SplitN(k, ":", 3)
		if len(parts) != 3 { continue }
		h := parseU64(parts[1])
		if h < hLimit {
			_ = db.Delete(it.Key(), nil)
		}
	}
}

func parseU64(s string) uint64 {
	var n uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' { break }
		n = n*10 + uint64(c-'0')
	}
	return n
}
