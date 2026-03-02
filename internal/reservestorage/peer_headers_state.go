package reservestorage

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
)

func keyPeerHdrFrom(peer string) []byte { return []byte("peerhdrfrom:" + peer) }

func GetPeerLastHeadersFrom(db *leveldb.DB, peer string) (uint64, bool, error) {
	b, err := db.Get(keyPeerHdrFrom(peer), nil)
	if err == nil && len(b)==8 { return binary.BigEndian.Uint64(b), true, nil }
	if errors.Is(err, leveldb.ErrNotFound) { return 0, false, nil }
	return 0, false, err
}

func SetPeerLastHeadersFrom(db *leveldb.DB, peer string, from uint64) error {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, from)
	return db.Put(keyPeerHdrFrom(peer), b, nil)
}

var _ = fmt.Sprintf
