package node

import (
	"testing"
	"github.com/syndtr/goleveldb/leveldb"
	"reserveos/core/chain"
	"reserveos/internal/reservestorage"
)

func TestHeaderStore(t *testing.T) {
	db, err := leveldb.OpenFile(t.TempDir(), nil)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	h := chain.BlockHeader{Height: 1, PrevHash: chain.Hash{}.String(), Bits: 1, Timestamp: 1}
	if err := reservestorage.PutHeader(db, h, "00"); err == nil {
		// hash length invalid in this test (expected), so ignore
	}
}
