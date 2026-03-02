package reservestorage

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"reserveos/core/chain"
)

func keyMempoolTx(id string) []byte { return []byte("mempool:tx:" + id) }

type memTx struct {
	Origin string `json:"origin"`

	Tx chain.Tx `json:"tx"`
	ExpiresUnix int64 `json:"expires_unix"`
}

func PutMempoolTx(db *leveldb.DB, tx chain.Tx, ttlSeconds int64) {
	m := memTx{Tx: tx, ExpiresUnix: time.Now().Unix() + ttlSeconds}
	b, _ := json.Marshal(m)
	_ = db.Put(keyMempoolTx(tx.ID()), b, nil)
}

func DelMempoolTx(db *leveldb.DB, txid string) {
	_ = db.Delete(keyMempoolTx(txid), nil)
}

func LoadMempoolTxs(db *leveldb.DB, limit int) ([]chain.Tx, error) {
	if limit <= 0 || limit > 10000 { limit = 10000 }
	it := db.NewIterator(util.BytesPrefix([]byte("mempool:tx:")), nil)
	defer it.Release()
	out := make([]chain.Tx, 0, 1024)
	now := time.Now().Unix()
	for it.Next() {
		var m memTx
		if err := json.Unmarshal(it.Value(), &m); err != nil { continue }
		if m.ExpiresUnix <= now {
			_ = db.Delete(it.Key(), nil)
			continue
		}
		out = append(out, m.Tx)
		if len(out) >= limit { break }
	}
	if err := it.Error(); err != nil && !errors.Is(err, leveldb.ErrNotFound) { return nil, err }
	return out, nil
}


func PutMempoolTxWithOrigin(db *leveldb.DB, tx chain.Tx, ttlSeconds int64, origin string) {
	m := memTx{Tx: tx, ExpiresUnix: time.Now().Unix() + ttlSeconds, Origin: origin}
	b, _ := json.Marshal(m)
	_ = db.Put(keyMempoolTx(tx.ID()), b, nil)
}
