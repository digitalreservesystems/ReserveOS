package reservestorage

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type Validator struct {
	PubkeyHex string `json:"pubkey_hex"`
	Name      string `json:"name"`
	Weight    int64  `json:"weight"`
	AddedUnix int64  `json:"added_unix"`
	Slashed   bool   `json:"slashed"`
}

func keyValidator(pub string) []byte { return []byte("pos:validator:" + pub) }

func PutValidator(db *leveldb.DB, v Validator) error {
	if v.AddedUnix == 0 { v.AddedUnix = time.Now().Unix() }
	b, err := json.Marshal(v)
	if err != nil { return err }
	return db.Put(keyValidator(v.PubkeyHex), b, nil)
}

func GetValidator(db *leveldb.DB, pub string) (Validator, bool, error) {
	b, err := db.Get(keyValidator(pub), nil)
	if err == nil {
		var v Validator
		if err := json.Unmarshal(b, &v); err != nil { return Validator{}, false, err }
		return v, true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) { return Validator{}, false, nil }
	return Validator{}, false, err
}

func ListValidators(db *leveldb.DB, limit int) ([]Validator, error) {
	if limit <= 0 || limit > 5000 { limit = 5000 }
	it := db.NewIterator(util.BytesPrefix([]byte("pos:validator:")), nil)
	defer it.Release()
	out := make([]Validator, 0, 16)
	for it.Next() {
		var v Validator
		if err := json.Unmarshal(it.Value(), &v); err != nil { return nil, err }
		out = append(out, v)
		if len(out) >= limit { break }
	}
	if err := it.Error(); err != nil { return nil, err }
	return out, nil
}

func SlashValidator(db *leveldb.DB, pub string) error {
	v, ok, err := GetValidator(db, pub)
	if err != nil { return err }
	if !ok { v = Validator{PubkeyHex: pub, Name: "unknown", Weight: 0} }
	v.Slashed = true
	return PutValidator(db, v)
}
