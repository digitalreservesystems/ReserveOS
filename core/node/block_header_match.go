package node

import (
	"bytes"
	"fmt"

	"reserveos/core/chain"
	"reserveos/internal/reservestorage"
)

func (n *Node) BlockMatchesStoredHeader(b *chain.Block) error {
	hash := b.Hash().String()
	hdr, ok, err := reservestorage.GetHeaderByHash(n.db.DB, hash)
	if err != nil { return err }
	if !ok { return fmt.Errorf("missing_header") }

	if hdr.Height != b.Header.Height { return fmt.Errorf("height_mismatch") }
	if hdr.PrevHash != b.Header.PrevHash { return fmt.Errorf("prev_mismatch") }
	if hdr.Bits != b.Header.Bits { return fmt.Errorf("bits_mismatch") }
	if hdr.Timestamp != b.Header.Timestamp { return fmt.Errorf("ts_mismatch") }
	if hdr.TxRoot != b.Header.TxRoot { return fmt.Errorf("txroot_mismatch") }

	// Optional: compare raw header serialization if present (not required)
	_ = bytes.Compare
	return nil
}
