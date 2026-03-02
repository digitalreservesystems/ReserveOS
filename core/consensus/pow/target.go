package pow

import (
	"encoding/binary"
	"math/big"
)

func CompactToBig(compact uint32) *big.Int {
	exponent := uint8(compact >> 24)
	mantissa := compact & 0x007fffff
	bn := new(big.Int).SetUint64(uint64(mantissa))
	if exponent <= 3 {
		shift := 8 * (3 - exponent)
		bn.Rsh(bn, uint(shift))
	} else {
		shift := 8 * (exponent - 3)
		bn.Lsh(bn, uint(shift))
	}
	return bn
}

func BigToCompact(target *big.Int) uint32 {
	if target.Sign() <= 0 { return 0 }
	b := target.Bytes()
	exponent := uint32(len(b))
	var mantissa uint32
	if exponent <= 3 {
		var tmp [4]byte
		copy(tmp[4-len(b):], b)
		mantissa = binary.BigEndian.Uint32(tmp[:]) >> 8
	} else {
		mantissa = uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
	}
	if mantissa&0x00800000 != 0 {
		mantissa >>= 8
		exponent++
	}
	return (exponent << 24) | (mantissa & 0x007fffff)
}

func LeadingZerosToTarget(lz uint32) *big.Int {
	if lz >= 256 { return big.NewInt(0) }
	one := big.NewInt(1)
	t := new(big.Int).Lsh(one, uint(256-lz))
	t.Sub(t, one)
	return t
}
