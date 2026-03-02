package reservestorage

import (
	"encoding/binary"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
)

func keyPeerHdrEnd(peer string) []byte { return []byte("peerhdrend:" + peer) }

func GetPeerLastHeadersEnd(db *leveldb.DB, peer string) (uint64, bool, error) {
	b, err := db.Get(keyPeerHdrEnd(peer), nil)
	if err == nil && len(b)==8 { return binary.BigEndian.Uint64(b), true, nil }
	if errors.Is(err, leveldb.ErrNotFound) { return 0, false, nil }
	return 0, false, err
}

func SetPeerLastHeadersEnd(db *leveldb.DB, peer string, end uint64) error {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, end)
	return db.Put(keyPeerHdrEnd(peer), b, nil)
}
