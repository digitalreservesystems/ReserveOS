package reservestorage

import (
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func ListPeerStats(db *leveldb.DB, limit int) ([]PeerStat, error) {
	if limit <= 0 || limit > 5000 { limit = 5000 }
	it := db.NewIterator(util.BytesPrefix([]byte("net:peerstat:")), nil)
	defer it.Release()
	out := make([]PeerStat, 0, 128)
	for it.Next() {
		var ps PeerStat
		if err := json.Unmarshal(it.Value(), &ps); err != nil { continue }
		out = append(out, ps)
		if len(out) >= limit { break }
	}
	if err := it.Error(); err != nil { return nil, err }
	return out, nil
}

func UnbanPeer(db *leveldb.DB, pub string) error {
	ps, _, _ := GetPeerStat(db, pub)
	ps.PubkeyHex = pub
	ps.BannedUntil = 0
	return PutPeerStat(db, ps)
}
