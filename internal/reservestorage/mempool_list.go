package reservestorage

import (
	"encoding/json"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func ListPersistedMempool(db *leveldb.DB, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 5000 { limit = 5000 }
	it := db.NewIterator(util.BytesPrefix([]byte("mempool:tx:")), nil)
	defer it.Release()
	out := make([]map[string]any, 0, 256)
	for it.Next() {
		var m map[string]any
		_ = json.Unmarshal(it.Value(), &m)
		out = append(out, m)
		if len(out) >= limit { break }
	}
	if err := it.Error(); err != nil { return nil, err }
	return out, nil
}
