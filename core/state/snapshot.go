package state

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type Snapshot struct {
	Version int `json:"version"`
	Height uint64 `json:"height"`
	TipHash string `json:"tip_hash"`
	CreatedUnix int64 `json:"created_unix"`
	Balances map[string]int64 `json:"balances"`
	Nonces map[string]uint64 `json:"nonces"`
}

func WriteSnapshot(db *leveldb.DB, dir string, height uint64, tipHash string) (string, error) {
	if dir == "" { dir = filepath.Join("runtime","snapshots") }
	if err := os.MkdirAll(dir, 0700); err != nil { return "", err }

	s := Snapshot{
		Version: 1,
		Height: height,
		TipHash: tipHash,
		CreatedUnix: time.Now().Unix(),
		Balances: map[string]int64{},
		Nonces: map[string]uint64{},
	}

	it := db.NewIterator(util.BytesPrefix([]byte("bal:")), nil)
	for it.Next() {
		k := string(it.Key())
		id := strings.TrimPrefix(k, "bal:")
		// stored as u64 big-endian in v1
		vBytes := it.Value()
		if len(vBytes) == 8 {
			u := uint64(vBytes[0])<<56 | uint64(vBytes[1])<<48 | uint64(vBytes[2])<<40 | uint64(vBytes[3])<<32 |
				uint64(vBytes[4])<<24 | uint64(vBytes[5])<<16 | uint64(vBytes[6])<<8 | uint64(vBytes[7])
			s.Balances[id] = int64(u)
		}
	}
	it.Release()

	it2 := db.NewIterator(util.BytesPrefix([]byte("nonce:")), nil)
	for it2.Next() {
		k := string(it2.Key())
		id := strings.TrimPrefix(k, "nonce:")
		vBytes := it2.Value()
		if len(vBytes) == 8 {
			u := uint64(vBytes[0])<<56 | uint64(vBytes[1])<<48 | uint64(vBytes[2])<<40 | uint64(vBytes[3])<<32 |
				uint64(vBytes[4])<<24 | uint64(vBytes[5])<<16 | uint64(vBytes[6])<<8 | uint64(vBytes[7])
			s.Nonces[id] = u
		}
	}
	it2.Release()

	name := filepath.Join(dir, "state_"+strconv.FormatUint(height,10)+".json.gz")
	f, err := os.Create(name)
	if err != nil { return "", err }
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	enc := json.NewEncoder(gz)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&s); err != nil { return "", err }
	return name, nil
}

func ListSnapshots(dir string) ([]string, error) {
	if dir == "" { dir = filepath.Join("runtime","snapshots") }
	ents, err := os.ReadDir(dir)
	if err != nil { return nil, err }
	out := make([]string, 0)
	for _, e := range ents {
		if e.IsDir() { continue }
		if strings.HasPrefix(e.Name(), "state_") && (strings.HasSuffix(e.Name(), ".json.gz") || strings.HasSuffix(e.Name(), ".gob.gz")) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func ReadSnapshot(path string) (Snapshot, error) {
	var s Snapshot
	f, err := os.Open(path)
	if err != nil { return s, err }
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil { return s, err }
	defer gz.Close()
	dec := json.NewDecoder(gz)
	err = dec.Decode(&s)
	return s, err
}

func RestoreSnapshot(db *leveldb.DB, s Snapshot) error {
	if err := ResetState(db); err != nil { return err }
	for id, bal := range s.Balances {
		_ = SetBalance(db, id, bal)
	}
	for id, n := range s.Nonces {
		_ = SetNonce(db, id, n)
	}
	return nil
}
