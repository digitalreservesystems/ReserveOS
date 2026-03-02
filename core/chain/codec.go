package chain

import (
	"bytes"
	"encoding/binary"
)

func putU32(b *bytes.Buffer, v uint32) { _ = binary.Write(b, binary.LittleEndian, v) }
func putU64(b *bytes.Buffer, v uint64) { _ = binary.Write(b, binary.LittleEndian, v) }
func putI64(b *bytes.Buffer, v int64)  { _ = binary.Write(b, binary.LittleEndian, v) }

func putBytes(b *bytes.Buffer, x []byte) {
	putU32(b, uint32(len(x)))
	b.Write(x)
}

func putString(b *bytes.Buffer, s string) { putBytes(b, []byte(s)) }

// CanonicalTxBytes encodes a Tx deterministically (no JSON ambiguity).
// IMPORTANT: Signatures are NOT included by default.
func CanonicalTxBytes(tx *Tx, includeSigs bool) []byte {
	var buf bytes.Buffer
	// domain tag
	buf.Write([]byte("RSVTX1"))
	putU32(&buf, tx.Version)
	putString(&buf, tx.Type)
	putString(&buf, tx.From)
	putString(&buf, tx.PubKey)
	putU64(&buf, tx.Nonce)
	putI64(&buf, tx.Fee)
	putString(&buf, tx.GasAsset)

	// outputs
	putU32(&buf, uint32(len(tx.Outputs)))
	for _, o := range tx.Outputs {
		putI64(&buf, o.Amount)
		putString(&buf, o.Asset)
		putString(&buf, o.Address)
		putString(&buf, o.P)
		putString(&buf, o.R)
		putString(&buf, o.Tag)
		putString(&buf, o.EncMemo)
		putU32(&buf, o.PolicyBits)
	}

	// otap claim
	if tx.OTAPClaim == nil {
		buf.WriteByte(0)
	} else {
		buf.WriteByte(1)
		putString(&buf, tx.OTAPClaim.P)
		putString(&buf, tx.OTAPClaim.R)
		putString(&buf, tx.OTAPClaim.Tag)
		putString(&buf, tx.OTAPClaim.To)
		putI64(&buf, tx.OTAPClaim.Amount)
		if includeSigs {
			putString(&buf, tx.OTAPClaim.ClaimSigHex)
		} else {
			putString(&buf, "")
		}
	}

	if includeSigs {
		putString(&buf, tx.SigHex)
	} else {
		putString(&buf, "")
	}
	return buf.Bytes()
}
