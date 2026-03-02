package reservestorage

import (
	"os"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"reserveos/core/chain"
)

func TestHeaderIndexForkAware(t *testing.T) {
	dir := t.TempDir()
	db, err := leveldb.OpenFile(dir, nil)
	if err != nil { t.Fatal(err) }
	defer db.Close()

	h1 := chain.BlockHeader{Height: 5, PrevHash: "aa", Bits: 1, Timestamp: 1}
	h2 := chain.BlockHeader{Height: 5, PrevHash: "bb", Bits: 1, Timestamp: 1}

	if err := PutHeader(db, h1, "h1"); err != nil { t.Fatal(err) }
	if err := PutHeader(db, h2, "h2"); err != nil { t.Fatal(err) }

	hs, err := ListHeaderHashesAtHeight(db, 5, 10)
	if err != nil { t.Fatal(err) }
	if len(hs) != 2 { t.Fatalf("expected 2 hashes, got %d (%v)", len(hs), hs) }

	_, ok, err := GetHeaderByHash(db, "h1")
	if err != nil || !ok { t.Fatalf("expected header by hash") }

	_ = os.ErrNotExist
}
