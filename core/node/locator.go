package node

import (
	"errors"

	"reserveos/internal/reservestorage"
)

// BuildBlockLocator returns a list of block hashes starting from the current tip,
// stepping back exponentially like Bitcoin's block locator.
// Uses the selected main-chain height mapping.
func (n *Node) BuildBlockLocator(max int) ([]string, error) {
	if max <= 0 { max = 32 }

	tipH, tipHash, ok, err := reservestorage.GetTip(n.db.DB)
	if err != nil || !ok { return nil, err }

	loc := make([]string, 0, max)
	step := uint64(1)
	h := tipH
	for len(loc) < max {
		// hash at height h
		var blk any
		okb, hashHex, err := reservestorage.GetBlockByHeight(n.db.DB, h, &blk)
		if err != nil || !okb { break }
		loc = append(loc, hashHex)
		if h == 0 { break }
		if len(loc) > 10 { step *= 2 } // exponential backoff after first few
		if step > h { h = 0 } else { h -= step }
	}

	// ensure tip hash is first even if GetBlockByHeight failed
	if len(loc) == 0 && tipHash != "" {
		loc = append(loc, tipHash)
	}
	return loc, nil
}

func (n *Node) FindFirstKnown(locator []string) (hash string, height uint64, ok bool, err error) {
	for _, hh := range locator {
		var b struct{ Header struct{ Height uint64 `json:"height"` } `json:"header"` }
		okb, err := reservestorage.GetBlockByHash(n.db.DB, hh, &b)
		if err != nil { return "", 0, false, err }
		if okb {
			return hh, b.Header.Height, true, nil
		}
	}
	return "", 0, false, nil
}

var _ = errors.New
