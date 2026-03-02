package reservestorage

import (
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
)

type HeaderView struct {
	Height uint64 `json:"height"`
	PrevHash struct{
		// Hash as hex string is stored in chain.Hash String() form
	} `json:"prev_hash"`
}
