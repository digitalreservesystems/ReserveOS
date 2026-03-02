package reservestorage

import (
	"encoding/json"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type Peer struct {
	Addr string `json:"addr"` // base URL e.g. http://1.2.3.4:18445
	AddedUnix int64 `json:"added_unix"`
}

func keyPeer(addr string) []byte { return []byte("net:peer:" + addr) }

func PutPeer(db *leveldb.DB, p Peer) error {
	b, err := json.Marshal(p)
	if err != nil { return err }
	return db.Put(keyPeer(p.Addr), b, nil)
}

func ListPeers(db *leveldb.DB, limit int) ([]Peer, error) {
	if limit <= 0 || limit > 5000 { limit = 5000 }
	it := db.NewIterator(util.BytesPrefix([]byte("net:peer:")), nil)
	defer it.Release()
	out := make([]Peer, 0, 64)
	for it.Next() {
		var p Peer
		if err := json.Unmarshal(it.Value(), &p); err != nil { return nil, err }
		out = append(out, p)
		if len(out) >= limit { break }
	}
	if err := it.Error(); err != nil { return nil, err }
	return out, nil
}

func HasPeer(db *leveldb.DB, addr string) (bool, error) {
	_, err := db.Get(keyPeer(addr), nil)
	if err == nil { return true, nil }
	if errors.Is(err, leveldb.ErrNotFound) { return false, nil }
	return false, err
}
