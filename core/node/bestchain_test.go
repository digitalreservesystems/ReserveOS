package node

import (
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"reserveos/core/chain"
	"reserveos/internal/reservestorage"
)

func TestBestChainMarker(t *testing.T) {
	db, err := leveldb.OpenFile(t.TempDir(), nil)
	if err != nil { t.Fatal(err) }
	defer db.Close()

	_ = reservestorage.SetBestChainTip(db, "abc")
	h, ok, err := reservestorage.GetBestChainTip(db)
	if err != nil || !ok || h != "abc" { t.Fatalf("expected best chain tip abc, got %v %v %v", h, ok, err) }

	_ = reservestorage.PutHeader(db, chain.BlockHeader{Height: 1, PrevHash: "gen", Bits: 1, Timestamp: 1}, "h1")
}
