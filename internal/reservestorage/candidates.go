package reservestorage

import (
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"strconv"
)

type Candidate struct {
	Height uint64 `json:"height"`
	Hash string `json:"hash"`
	PrevHash string `json:"prev_hash"`
}

func keyCand(height uint64, hash string) []byte {
	return []byte("cand:" + strconv.FormatUint(height,10) + ":" + hash)
}

func PutCandidate(db *leveldb.DB, c Candidate) {
	b, _ := json.Marshal(c)
	_ = db.Put(keyCand(c.Height, c.Hash), b, nil)
}

func DelCandidate(db *leveldb.DB, height uint64, hash string) {
	_ = db.Delete(keyCand(height, hash), nil)
}

func ListCandidatesAtHeight(db *leveldb.DB, height uint64, limit int) ([]Candidate, error) {
	if limit <= 0 || limit > 2000 { limit = 2000 }
	prefix := []byte("cand:" + strconv.FormatUint(height,10) + ":")
	it := db.NewIterator(util.BytesPrefix(prefix), nil)
	defer it.Release()
	out := make([]Candidate, 0, 8)
	for it.Next() {
		var c Candidate
		_ = json.Unmarshal(it.Value(), &c)
		out = append(out, c)
		if len(out) >= limit { break }
	}
	if err := it.Error(); err != nil { return nil, err }
	return out, nil
}
