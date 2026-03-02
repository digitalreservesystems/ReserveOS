package reservestorage

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

type PoPEvent struct {
	ID string `json:"id"`
	Type string `json:"type"`
	Delta uint64 `json:"delta"`
	TimeUnix int64 `json:"time_unix"`
}

func PutPoPEvent(db *leveldb.DB, ev PoPEvent) error {
	if ev.TimeUnix == 0 { ev.TimeUnix = time.Now().Unix() }
	b, err := json.Marshal(ev)
	if err != nil { return err }
	k := []byte(fmt.Sprintf("pop:event:%d:%s:%s", ev.TimeUnix, ev.ID, ev.Type))
	return db.Put(k, b, nil)
}
