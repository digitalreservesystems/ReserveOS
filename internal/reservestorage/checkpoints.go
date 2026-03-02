package reservestorage

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"reserveos/core/consensus/finality"
)

func keyCheckpoint(height uint64) []byte { return []byte(fmt.Sprintf("finality:checkpoint:%d", height)) }

func PutCheckpoint(db *leveldb.DB, height uint64, cp finality.Checkpoint) error {
	b, err := json.Marshal(cp)
	if err != nil { return err }
	return db.Put(keyCheckpoint(height), b, nil)
}

func GetCheckpoint(db *leveldb.DB, height uint64) (finality.Checkpoint, bool, error) {
	b, err := db.Get(keyCheckpoint(height), nil)
	if err == nil {
		var cp finality.Checkpoint
		if err := json.Unmarshal(b, &cp); err != nil { return finality.Checkpoint{}, false, err }
		return cp, true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) { return finality.Checkpoint{}, false, nil }
	return finality.Checkpoint{}, false, err
}

func ListCheckpoints(db *leveldb.DB, limit int) ([]finality.Checkpoint, error) {
	if limit <= 0 || limit > 5000 { limit = 5000 }
	it := db.NewIterator(util.BytesPrefix([]byte("finality:checkpoint:")), nil)
	defer it.Release()
	out := make([]finality.Checkpoint, 0, 128)
	for it.Next() {
		var cp finality.Checkpoint
		if err := json.Unmarshal(it.Value(), &cp); err != nil { return nil, err }
		out = append(out, cp)
		if len(out) >= limit { break }
	}
	if err := it.Error(); err != nil { return nil, err }
	return out, nil
}
