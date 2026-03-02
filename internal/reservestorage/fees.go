package reservestorage

import (
	"encoding/binary"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
)

var (
	keyFeePoolValidators    = []byte("fees:pool:validators")
	keyFeePoolParticipation = []byte("fees:pool:participation")
	keyFeePoolTreasury      = []byte("fees:pool:treasury")
	keyFeePoolDefense       = []byte("fees:pool:defense")
)

func addU64(db *leveldb.DB, key []byte, add uint64) error {
	cur, _ := getU64(db, key)
	next := cur + add
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, next)
	return db.Put(key, b, nil)
}

func getU64(db *leveldb.DB, key []byte) (uint64, bool) {
	b, err := db.Get(key, nil)
	if err == nil {
		return binary.BigEndian.Uint64(b), true
	}
	if errors.Is(err, leveldb.ErrNotFound) {
		return 0, false
	}
	return 0, false
}

func AddFeePools(db *leveldb.DB, v, pop, tr, def uint64) error {
	if v > 0 { _ = addU64(db, keyFeePoolValidators, v) }
	if pop > 0 { _ = addU64(db, keyFeePoolParticipation, pop) }
	if tr > 0 { _ = addU64(db, keyFeePoolTreasury, tr) }
	if def > 0 { _ = addU64(db, keyFeePoolDefense, def) }
	return nil
}

func GetFeePools(db *leveldb.DB) map[string]uint64 {
	out := map[string]uint64{}
	if v, ok := getU64(db, keyFeePoolValidators); ok { out["validators"] = v }
	if v, ok := getU64(db, keyFeePoolParticipation); ok { out["participation"] = v }
	if v, ok := getU64(db, keyFeePoolTreasury); ok { out["treasury"] = v }
	if v, ok := getU64(db, keyFeePoolDefense); ok { out["defense"] = v }
	return out
}


func SetFeePool(db *leveldb.DB, name string, value uint64) error {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, value)
	switch name {
	case "validators":
		return db.Put(keyFeePoolValidators, b, nil)
	case "participation":
		return db.Put(keyFeePoolParticipation, b, nil)
	case "treasury":
		return db.Put(keyFeePoolTreasury, b, nil)
	case "defense":
		return db.Put(keyFeePoolDefense, b, nil)
	default:
		return nil
	}
}

func DrainFeePool(db *leveldb.DB, name string, amount uint64) (drained uint64, remaining uint64, err error) {
	pools := GetFeePools(db)
	cur := pools[name]
	if cur == 0 { return 0, 0, nil }
	if amount == 0 || amount > cur { amount = cur }
	drained = amount
	remaining = cur - amount
	err = SetFeePool(db, name, remaining)
	return drained, remaining, err
}
