package reservestorage

import (
	"encoding/binary"
	"errors"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

func keyPeerGetBlock(peer, hash string) []byte { return []byte("peergetblk:"+peer+":"+hash) }

func MarkPeerGetBlock(db *leveldb.DB, peer, hash string) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(time.Now().Unix()))
	_ = db.Put(keyPeerGetBlock(peer, hash), b, nil)
}

func PeerGetBlockRecently(db *leveldb.DB, peer, hash string, withinSec int64) bool {
	b, err := db.Get(keyPeerGetBlock(peer, hash), nil)
	if err != nil { return false }
	if len(b)!=8 { return false }
	ts := int64(binary.BigEndian.Uint64(b))
	return time.Now().Unix()-ts <= withinSec
}

var _ = errors.Is
