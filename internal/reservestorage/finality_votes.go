package reservestorage

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type VoteRecord struct {
	Height    uint64 `json:"height"`
	BlockHash string `json:"block_hash"`
	ChainID   string `json:"chain_id"`
	PubkeyHex string `json:"pubkey_hex"`
	SigHex    string `json:"sig_hex"`
	TimeUnix  int64  `json:"time_unix"`
}

func keyVote(height uint64, pubkeyHex string) []byte {
	return []byte(fmt.Sprintf("finality:vote:%d:%s", height, pubkeyHex))
}

func PutVote(db *leveldb.DB, v VoteRecord) error {
	b, err := json.Marshal(v)
	if err != nil { return err }
	return db.Put(keyVote(v.Height, v.PubkeyHex), b, nil)
}

func HasVote(db *leveldb.DB, height uint64, pubkeyHex string) (bool, error) {
	_, err := db.Get(keyVote(height, pubkeyHex), nil)
	if err == nil { return true, nil }
	if errors.Is(err, leveldb.ErrNotFound) { return false, nil }
	return false, err
}

func ListVotesForHeight(db *leveldb.DB, height uint64) ([]VoteRecord, error) {
	prefix := []byte(fmt.Sprintf("finality:vote:%d:", height))
	it := db.NewIterator(util.BytesPrefix(prefix), nil)
	defer it.Release()
	var out []VoteRecord
	for it.Next() {
		var vr VoteRecord
		if err := json.Unmarshal(it.Value(), &vr); err != nil { return nil, err }
		out = append(out, vr)
	}
	if err := it.Error(); err != nil { return nil, err }
	return out, nil
}
