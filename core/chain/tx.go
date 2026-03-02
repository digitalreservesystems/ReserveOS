package chain

import (
	"crypto/sha256"
	"encoding/binary"
)

type Tx struct {
	From    string `json:"from"` // account id (ed25519 pubkey hex) for account-style txs
	PubKey  string `json:"pubkey"` // ed25519 pubkey hex (must match From)
	SigHex  string `json:"sig"` // ed25519 signature hex over tx signing bytes
	Type    string `json:"type"` // "transfer" (default) or "otap_claim"
	OTAPClaim *OTAPClaim `json:"otap_claim,omitempty"`

	Version uint32  `json:"version"`
	Nonce   uint64  `json:"nonce"`
	Fee     int64   `json:"fee"`
	GasAsset string `json:"gas_asset"`
	Outputs []TxOut `json:"outputs"`
	Memo    string  `json:"memo,omitempty"`
}

type TxOut struct {
	Amount int64  `json:"amount"`
	Asset  string `json:"asset"`

	// Legacy/plain address support (v0)
	Address string `json:"address,omitempty"`

	// OTAP fields (v1)
	P        string `json:"P,omitempty"`        // one-time destination pubkey (compressed hex)
	R        string `json:"R,omitempty"`        // ephemeral pubkey (compressed hex)
	Tag      string `json:"tag,omitempty"`      // optional hint tag (hex, e.g. 4 chars for 16-bit)
	EncMemo  string `json:"enc_memo,omitempty"` // base64(nonce||ciphertext)
	PolicyBits uint32 `json:"policy_bits,omitempty"`
}

func (tx *Tx) SigningBytes() []byte {
	return CanonicalTxBytes(tx, false)
}

func (tx *Tx) ID() string {
	b := tx.SigningBytes()
	h := hash256(b)
	return h.String()
}


func hex32(b []byte) string {
	const hexd = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i := 0; i < len(b); i++ {
		out[i*2] = hexd[b[i]>>4]
		out[i*2+1] = hexd[b[i]&0x0f]
	}
	return string(out)
}
