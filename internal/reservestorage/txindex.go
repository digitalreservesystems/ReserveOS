package reservestorage

import (
	"encoding/json"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
	"reserveos/core/chain"
)

func keyTx(txid string) []byte { return []byte("tx:" + txid) }

func PutTx(db *leveldb.DB, tx chain.Tx) error {
	b, err := json.Marshal(tx)
	if err != nil { return err }
	return db.Put(keyTx(tx.ID()), b, nil)
}

func GetTx(db *leveldb.DB, txid string) (chain.Tx, bool, error) {
	b, err := db.Get(keyTx(txid), nil)
	if err == nil {
		var tx chain.Tx
		if err := json.Unmarshal(b, &tx); err != nil { return chain.Tx{}, false, err }
		return tx, true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) { return chain.Tx{}, false, nil }
	return chain.Tx{}, false, err
}
