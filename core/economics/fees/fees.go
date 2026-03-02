package fees

import (
	"crypto/sha256"
)

type Mode string

const (
	ModeNormal     Mode = "NORMAL"
	ModeDefense    Mode = "DEFENSE"
	ModeContainment Mode = "CONTAINMENT"
)

type Params struct {
	GasAsset string
	BaseFeeMin int64 // minimum fee per tx unit
	PerByte int64    // fee per byte unit
	OTAPMultiplier int64 // e.g. 10 means 10x
	Mode Mode
}

type Split struct {
	ToValidators int64
	ToParticipation int64
	ToTreasury int64
	ToDefensePool int64
}

func EstimateFee(txBytes int, isOTAP bool, p Params) (fee int64) {
	if txBytes < 0 { txBytes = 0 }
	units := int64(txBytes)
	fee = p.BaseFeeMin + p.PerByte*units
	if isOTAP && p.OTAPMultiplier > 1 {
		fee *= p.OTAPMultiplier
	}
	// mode multiplier (simple demo): DEFENSE x2, CONTAINMENT x4
	switch p.Mode {
	case ModeDefense:
		fee *= 2
	case ModeContainment:
		fee *= 4
	}
	if fee < p.BaseFeeMin { fee = p.BaseFeeMin }
	return fee
}

func SplitFees(total int64, p Params) Split {
	if total <= 0 { return Split{} }
	// Conservative default splits:
	// NORMAL: 50% validators, 20% participation(PoP), 30% treasury
	// DEFENSE: 35% validators, 15% participation, 20% treasury, 30% defense pool
	// CONTAINMENT: 25% validators, 10% participation, 15% treasury, 50% defense pool
	var v,pop,tr,def int64
	switch p.Mode {
	case ModeDefense:
		v = total*35/100
		pop = total*15/100
		tr = total*20/100
		def = total - v - pop - tr
	case ModeContainment:
		v = total*25/100
		pop = total*10/100
		tr = total*15/100
		def = total - v - pop - tr
	default:
		v = total*50/100
		pop = total*20/100
		tr = total - v - pop
		def = 0
	}
	return Split{ToValidators: v, ToParticipation: pop, ToTreasury: tr, ToDefensePool: def}
}

// TxSizeHint is a stable size approximation for fee purposes (not JSON length).
// For v1 demo we hash the tx ID bytes and use fixed-size estimate: 180 + 80*outputs.
func TxSizeHint(txid string, outputs int) int {
	_ = sha256.Sum256([]byte(txid))
	if outputs < 0 { outputs = 0 }
	return 180 + 80*outputs
}
