package pop

import (
	"time"

	"reserveos/core/state"
	"reserveos/internal/reservestorage"
	"github.com/syndtr/goleveldb/leveldb"
)

type Params struct {
	EpochBlocks uint64
	MinPayout uint64
}

func DefaultParams() Params {
	return Params{EpochBlocks: 50, MinPayout: 100}
}

// DistributeIfEpoch distributes the participation fee pool at epoch boundaries.
// Credits balances under ids: "pop:<participant_id>".
func DistributeIfEpoch(db *leveldb.DB, height uint64, params Params) (paid uint64, err error) {
	if params.EpochBlocks == 0 { params = DefaultParams() }
	if height == 0 || height%params.EpochBlocks != 0 { return 0, nil }

	drain, _, err := reservestorage.DrainFeePool(db, "participation", 0)
	if err != nil { return 0, err }
	if drain < params.MinPayout { 
		// put it back (no payout)
		_, _, _ = reservestorage.DrainFeePool(db, "participation", 0) // no-op
		_ = reservestorage.SetFeePool(db, "participation", drain) // restore
		return 0, nil
	}

	parts, err := reservestorage.ListParticipants(db, 5000)
	if err != nil { return 0, err }
	if len(parts) == 0 { _ = reservestorage.SetFeePool(db, "participation", drain); return 0, nil }
	// Use score weights (heartbeats) if present, otherwise participant.Weight.
	weights := make([]int64, len(parts))
	var totalW int64
	for i, p := range parts {
		if p.Weight <= 0 { continue }
		sc, _ := reservestorage.GetScore(db, p.ID)
		w := int64(sc)
		if w <= 0 { w = p.Weight }
		weights[i] = w
		totalW += w
	}
	if totalW <= 0 { _ = reservestorage.SetFeePool(db, "participation", drain); return 0, nil }
	if totalW <= 0 {
		_ = reservestorage.SetFeePool(db, "participation", drain)
		return 0, nil
	}

	remaining := int64(drain)
	for i, p := range parts {
		if p.Weight <= 0 { continue }
		share := int64(drain) * weights[i] / totalW
		if i == len(parts)-1 { share = remaining } // remainder
		if share <= 0 { continue }
		remaining -= share
		_, err := state.AddBalance(db, "pop:"+p.ID, share)
		if err != nil { return uint64(int64(drain)-remaining), err }
	}
	_ = time.Now()
	_ = reservestorage.ResetScores(db)
	return drain, nil
}
