package finality

type Checkpoint struct {
	Height   uint64 `json:"height"`
	HashHex  string `json:"hash"`
	TimeUnix int64  `json:"time_unix"`
}
