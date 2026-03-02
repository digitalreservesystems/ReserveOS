package chain

import (
	"crypto/sha256"
	"encoding/binary"
)

type Hash [32]byte

func (h Hash) String() string {
	const hexd = "0123456789abcdef"
	out := make([]byte, 64)
	for i := 0; i < 32; i++ {
		b := h[i]
		out[i*2] = hexd[b>>4]
		out[i*2+1] = hexd[b&0x0f]
	}
	return string(out)
}

func sha256d(b []byte) Hash {
	h1 := sha256.Sum256(b)
	h2 := sha256.Sum256(h1[:])
	return h2
}

type BlockHeader struct {
	Version   uint32 `json:"version"`
	ChainID   string `json:"chain_id"`
	Height    uint64 `json:"height"`
	TimeUnix  int64  `json:"time_unix"`
	PrevHash  Hash   `json:"prev_hash"`
	StateRoot Hash   `json:"state_root"`
	TxRoot    Hash   `json:"tx_root"`
	Bits      uint32 `json:"bits"`
	Nonce     uint64 `json:"nonce"`
}

func (h *BlockHeader) BytesForHash() []byte {
	chainIDb := []byte(h.ChainID)
	buf := make([]byte, 0, 4+4+len(chainIDb)+8+8+32+32+32+4+8)
	tmp4 := make([]byte, 4)
	tmp8 := make([]byte, 8)

	binary.LittleEndian.PutUint32(tmp4, h.Version); buf = append(buf, tmp4...)
	binary.LittleEndian.PutUint32(tmp4, uint32(len(chainIDb))); buf = append(buf, tmp4...)
	buf = append(buf, chainIDb...)
	binary.LittleEndian.PutUint64(tmp8, h.Height); buf = append(buf, tmp8...)
	binary.LittleEndian.PutUint64(tmp8, uint64(h.TimeUnix)); buf = append(buf, tmp8...)
	buf = append(buf, h.PrevHash[:]...)
	buf = append(buf, h.StateRoot[:]...)
	buf = append(buf, h.TxRoot[:]...)
	binary.LittleEndian.PutUint32(tmp4, h.Bits); buf = append(buf, tmp4...)
	binary.LittleEndian.PutUint64(tmp8, h.Nonce); buf = append(buf, tmp8...)
	return buf
}

func (h *BlockHeader) Hash() Hash { return sha256d(h.BytesForHash()) }

type Block struct {
	Header BlockHeader `json:"header"`
	Txs    []Tx        `json:"txs"`
}


func (b *Block) SizeHint() int {
	sz := 256 + len(b.Txs)*128
	for _, tx := range b.Txs { sz += len(tx.Outputs)*64 }
	return sz
}
