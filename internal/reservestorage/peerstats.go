package reservestorage

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

type PeerStat struct {
	PubkeyHex    string `json:"pubkey_hex"`
	Score        int64  `json:"score"`
	BannedUntil  int64  `json:"banned_until"`
	UpdatedUnix  int64  `json:"updated_unix"`
}

func keyPeerStat(pub string) []byte { return []byte("net:peerstat:" + pub) }

func GetPeerStat(db *leveldb.DB, pub string) (PeerStat, bool, error) {
	b, err := db.Get(keyPeerStat(pub), nil)
	if err == nil {
		var ps PeerStat
		if err := json.Unmarshal(b, &ps); err != nil { return PeerStat{}, false, err }
		return ps, true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) { return PeerStat{PubkeyHex: pub}, false, nil }
	return PeerStat{}, false, err
}

func PutPeerStat(db *leveldb.DB, ps PeerStat) error {
	ps.UpdatedUnix = time.Now().Unix()
	b, err := json.Marshal(ps)
	if err != nil { return err }
	return db.Put(keyPeerStat(ps.PubkeyHex), b, nil)
}

func AddPeerScore(db *leveldb.DB, pub string, delta int64) (PeerStat, error) {
	ps, _, _ := GetPeerStat(db, pub)
	ps.PubkeyHex = pub
	ps.Score += delta
	_ = PutPeerStat(db, ps)
	return ps, nil
}

func BanPeer(db *leveldb.DB, pub string, seconds int64) error {
	ps, _, _ := GetPeerStat(db, pub)
	ps.PubkeyHex = pub
	ps.BannedUntil = time.Now().Unix() + seconds
	return PutPeerStat(db, ps)
}

func IsBanned(db *leveldb.DB, pub string) (bool, int64) {
	ps, ok, _ := GetPeerStat(db, pub)
	if !ok { return false, 0 }
	now := time.Now().Unix()
	if ps.BannedUntil > now { return true, ps.BannedUntil }
	return false, 0
}
