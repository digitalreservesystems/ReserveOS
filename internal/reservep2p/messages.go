package reservep2p

import "encoding/json"

// Msg is a small typed envelope for P2P frames.
type Msg struct {
	Type string `json:"type"` // inv_tx|get_tx|tx|inv_block|get_block|block
	ID   string `json:"id,omitempty"`
	Hash string `json:"hash,omitempty"`
	Data []byte `json:"data,omitempty"`
}

func EncodeMsg(m Msg) []byte { b,_ := json.Marshal(m); return b }
func DecodeMsg(b []byte) (Msg, error) { var m Msg; err := json.Unmarshal(b,&m); return m, err }
