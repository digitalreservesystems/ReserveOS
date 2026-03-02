package chain

import (
	"crypto/sha256"
	"encoding/binary"

	"reserveos/core/consensus/pow"
)

func GenesisBlock(g *Genesis) *Block {
	var zero Hash
	sr := genesisStateRootPlaceholder(g)
	t := pow.LeadingZerosToTarget(g.Consensus.PoWLeadingZeroBits)
	bits := pow.BigToCompact(t)

	h := BlockHeader{
		Version:   uint32(g.ProtocolVersion),
		ChainID:   g.ChainID,
		Height:    0,
		TimeUnix:  g.GenesisTimeUnix,
		PrevHash:  zero,
		StateRoot: sr,
		TxRoot:    sha256d([]byte("genesis:txroot:none")),
		Bits:      bits,
		Nonce:     0,
	}
	return &Block{Header: h, Txs: nil}
}

func genesisStateRootPlaceholder(g *Genesis) Hash {
	h := sha256.New()
	h.Write([]byte("reserveos:genesis:stateroot:v0"))
	h.Write([]byte(g.ChainID))

	tmp8 := make([]byte, 8)
	for _, a := range g.Allocations {
		h.Write([]byte(a.Address))
		h.Write([]byte{0})
		h.Write([]byte(a.Balance))
		h.Write([]byte{0})
	}
	binary.LittleEndian.PutUint64(tmp8, uint64(g.Consensus.PoWLeadingZeroBits))
	h.Write(tmp8)

	sum := h.Sum(nil)
	var out Hash
	copy(out[:], sum[:32])
	return out
}
