package chain

import (
	"encoding/json"
	"errors"
	"os"
)

type Genesis struct {
	ChainID         string `json:"chain_id"`
	Network         string `json:"network"`
	GenesisTimeUnix int64  `json:"genesis_time_unix"`
	ProtocolVersion uint32 `json:"protocol_version"`

	Consensus struct {
		PoWLeadingZeroBits uint32 `json:"pow_leading_zero_bits"`
	} `json:"consensus"`

	// Optional PoS finality parameters (for checkpoint voting)
	Finality *struct {
		CheckpointInterval uint64 `json:"checkpoint_interval"`
		ThresholdNum       int64  `json:"threshold_num"`
		ThresholdDen       int64  `json:"threshold_den"`
	} `json:"finality,omitempty"`

	// Optional validator set (for PoS finality)
	Validators []struct {
		Name      string `json:"name"`
		PubkeyHex string `json:"pubkey_hex"`
		Weight    int64  `json:"weight"`
	} `json:"validators,omitempty"`

	Allocations []struct {
		Tag     string `json:"tag"`
		Address string `json:"address"`
		Balance string `json:"balance"`
	} `json:"allocations"`

	Params map[string]any `json:"params"`
}

func LoadGenesis(path string) (*Genesis, error) {
	b, err := os.ReadFile(path)
	if err != nil { return nil, err }
	var g Genesis
	if err := json.Unmarshal(b, &g); err != nil { return nil, err }
	if err := g.Validate(); err != nil { return nil, err }
	return &g, nil
}

func (g *Genesis) Validate() error {
	if g.ChainID == "" { return errors.New("genesis.chain_id required") }
	if g.ProtocolVersion == 0 { return errors.New("genesis.protocol_version required") }
	if g.GenesisTimeUnix <= 0 { return errors.New("genesis.genesis_time_unix required") }
	if g.Consensus.PoWLeadingZeroBits == 0 || g.Consensus.PoWLeadingZeroBits > 255 {
		return errors.New("genesis.consensus.pow_leading_zero_bits invalid")
	}
	for i, a := range g.Allocations {
		if a.Address == "" { return errors.New("genesis.allocations[" + itoa(i) + "].address required") }
		if a.Balance == "" { return errors.New("genesis.allocations[" + itoa(i) + "].balance required") }
	}
	// if finality present, basic sanity
	if g.Finality != nil {
		if g.Finality.CheckpointInterval == 0 { return errors.New("genesis.finality.checkpoint_interval invalid") }
		if g.Finality.ThresholdNum <= 0 || g.Finality.ThresholdDen <= 0 { return errors.New("genesis.finality.threshold invalid") }
	}
	return nil
}

func itoa(i int) string {
	if i == 0 { return "0" }
	neg := false
	if i < 0 { neg = true; i = -i }
	var buf [20]byte
	n := len(buf)
	for i > 0 { n--; buf[n] = byte('0' + (i % 10)); i /= 10 }
	if neg { n--; buf[n] = '-' }
	return string(buf[n:])
}
