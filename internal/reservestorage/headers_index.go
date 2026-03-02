package reservestorage

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"reserveos/core/chain"
	"strings"
)

func keyHdrHash(hash string) []byte { return []byte("hdr:" + hash) } // hash->header json
func keyHdrAtHeight(h uint64, hash string) []byte { return []byte(fmt.Sprintf("hdrh:%d:%s", h, hash)) } // height->hash membership

var keyHeaderTipH = []byte("hdrtip:h")
var keyHeaderTipHash = []byte("hdrtip:hash")

func PutHeader(db *leveldb.DB, header chain.BlockHeader, hash string) error {
	b, _ := json.Marshal(header)
	if err := db.Put(keyHdrHash(hash), b, nil); err != nil { return err }
	// record membership at height (does not overwrite other forks)
	_ = db.Put(keyHdrAtHeight(header.Height, hash), []byte{1}, nil)
	return nil
}

func GetHeaderByHash(db *leveldb.DB, hash string) (chain.BlockHeader, bool, error) {
	var hdr chain.BlockHeader
	b, err := db.Get(keyHdrHash(hash), nil)
	if err == nil {
		if err := json.Unmarshal(b, &hdr); err != nil { return hdr, false, err }
		return hdr, true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) { return hdr, false, nil }
	return hdr, false, err
}

func ListHeaderHashesAtHeight(db *leveldb.DB, h uint64, limit int) ([]string, error) {
	if limit <= 0 || limit > 5000 { limit = 5000 }
	prefix := []byte(fmt.Sprintf("hdrh:%d:", h))
	it := db.NewIterator(util.BytesPrefix(prefix), nil)
	defer it.Release()
	out := make([]string, 0, 8)
	for it.Next() {
		k := string(it.Key())
		// hdrh:<h>:<hash>
		parts := strings.SplitN(k, ":", 3)
		if len(parts) == 3 { out = append(out, parts[2]) }
		if len(out) >= limit { break }
	}
	if err := it.Error(); err != nil { return nil, err }
	return out, nil
}

func SetHeaderTip(db *leveldb.DB, h uint64, hash string) error {
	hb := make([]byte, 8)
	binary.BigEndian.PutUint64(hb, h)
	if err := db.Put(keyHeaderTipH, hb, nil); err != nil { return err }
	return db.Put(keyHeaderTipHash, []byte(hash), nil)
}

func GetHeaderTip(db *leveldb.DB) (uint64, string, bool, error) {
	hb, err := db.Get(keyHeaderTipH, nil)
	if err == nil && len(hb)==8 {
		h := binary.BigEndian.Uint64(hb)
		hashb, _ := db.Get(keyHeaderTipHash, nil)
		return h, string(hashb), true, nil
	}
	if errors.Is(err, leveldb.ErrNotFound) { return 0, "", false, nil }
	return 0, "", false, err
}
