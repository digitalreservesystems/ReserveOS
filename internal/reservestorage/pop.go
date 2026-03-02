package reservestorage

import (
	"encoding/json"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type Participant struct {
	ID     string `json:"id"`     // e.g. wallet id, node id, account id
	Weight int64  `json:"weight"` // >=1
	AddedAt int64 `json:"added_at"`
}

func keyParticipant(id string) []byte { return []byte("pop:participant:" + id) }

func PutParticipant(db *leveldb.DB, p Participant) error {
	b, err := json.Marshal(p)
	if err != nil { return err }
	return db.Put(keyParticipant(p.ID), b, nil)
}

func GetParticipant(db *leveldb.DB, id string) (Participant, bool, error) {
	b, err := db.Get(keyParticipant(id), nil)
	if err == nil {
		var out Participant
		if err := json.Unmarshal(b, &out); err != nil { return Participant{}, false, err }
		return out, true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) { return Participant{}, false, nil }
	return Participant{}, false, err
}

func ListParticipants(db *leveldb.DB, limit int) ([]Participant, error) {
	if limit <= 0 || limit > 5000 { limit = 5000 }
	it := db.NewIterator(util.BytesPrefix([]byte("pop:participant:")), nil)
	defer it.Release()
	out := make([]Participant, 0)
	for it.Next() {
		var p Participant
		if err := json.Unmarshal(it.Value(), &p); err != nil { return nil, err }
		out = append(out, p)
		if len(out) >= limit { break }
	}
	if err := it.Error(); err != nil { return nil, err }
	return out, nil
}
