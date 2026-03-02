package state

import (
	"compress/gzip"
	"encoding/gob"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func WriteSnapshotBin(db *leveldb.DB, dir string, height uint64, tipHash string) (string, error) {
	if dir == "" { dir = filepath.Join("runtime","snapshots") }
	if err := os.MkdirAll(dir, 0700); err != nil { return "", err }

	bal, non, err := ExportStateMaps(db)
	if err != nil { return "", err }

	s := Snapshot{
		Version: 2,
		Height: height,
		TipHash: tipHash,
		CreatedUnix: time.Now().Unix(),
		Balances: bal,
		Nonces: non,
	}

	name := filepath.Join(dir, "state_"+strconv.FormatUint(height,10)+".gob.gz")
	f, err := os.Create(name)
	if err != nil { return "", err }
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	enc := gob.NewEncoder(gz)
	if err := enc.Encode(&s); err != nil { return "", err }
	return name, nil
}

func ReadSnapshotBin(path string) (Snapshot, error) {
	var s Snapshot
	f, err := os.Open(path)
	if err != nil { return s, err }
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil { return s, err }
	defer gz.Close()
	dec := gob.NewDecoder(gz)
	err = dec.Decode(&s)
	return s, err
}

// ExportStateMaps scans LevelDB prefixes bal: and nonce: and returns maps.
func ExportStateMaps(db *leveldb.DB) (map[string]int64, map[string]uint64, error) {
	bal := map[string]int64{}
	non := map[string]uint64{}

	it := db.NewIterator(util.BytesPrefix([]byte("bal:")), nil)
	for it.Next() {
		k := string(it.Key())
		id := strings.TrimPrefix(k, "bal:")
		v := it.Value()
		if len(v) == 8 {
			u := uint64(v[0])<<56 | uint64(v[1])<<48 | uint64(v[2])<<40 | uint64(v[3])<<32 |
				uint64(v[4])<<24 | uint64(v[5])<<16 | uint64(v[6])<<8 | uint64(v[7])
			bal[id] = int64(u)
		}
	}
	it.Release()
	if err := it.Error(); err != nil { return nil, nil, err }

	it2 := db.NewIterator(util.BytesPrefix([]byte("nonce:")), nil)
	for it2.Next() {
		k := string(it2.Key())
		id := strings.TrimPrefix(k, "nonce:")
		v := it2.Value()
		if len(v) == 8 {
			u := uint64(v[0])<<56 | uint64(v[1])<<48 | uint64(v[2])<<40 | uint64(v[3])<<32 |
				uint64(v[4])<<24 | uint64(v[5])<<16 | uint64(v[6])<<8 | uint64(v[7])
			non[id] = u
		}
	}
	it2.Release()
	if err := it2.Error(); err != nil { return nil, nil, err }

	return bal, non, nil
}
