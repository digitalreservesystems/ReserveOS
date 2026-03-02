package node

import "errors"

type FinalityValidator struct {
	Name      string `json:"name"`
	PubkeyHex string `json:"pubkey_hex"` // ed25519 public key hex (32 bytes) or "AUTO_FROM_KEYVAULT"
	Weight    int64  `json:"weight"`
}

type FinalityConfig struct {
	Enabled               bool                `json:"enabled"`
	CheckpointInterval    uint64              `json:"checkpoint_interval"`
	ThresholdNum          int64               `json:"threshold_num"`
	ThresholdDen          int64               `json:"threshold_den"`
	Validators            []FinalityValidator `json:"validators"`
	Peers                 []string            `json:"peers"` // base URLs
	GossipIntervalSeconds int                 `json:"gossip_interval_seconds"`
}

type SyncConfig struct {
	Enabled          bool     `json:"enabled"`
	Peers            []string `json:"peers"` // base URLs
	IntervalSeconds  int      `json:"interval_seconds"`
	MaxBlocksPerRound int     `json:"max_blocks_per_round"`
}

type PoPConfig struct {
	EpochBlocks uint64 `json:"epoch_blocks"`
	MinPayout uint64 `json:"min_payout"`
}

type MempoolConfig struct {
	MaxTxsPerSender int `json:"max_txs_per_sender"`
	MaxTxs int `json:"max_txs"`
	MaxBytes int `json:"max_bytes"`
	TxTTLSeconds int `json:"tx_ttl_seconds"`
	AllowFutureNonces bool `json:"allow_future_nonces"`
	MaxNonceGap uint64 `json:"max_nonce_gap"`
	RBFMinFeeBump float64 `json:"rbf_min_fee_bump"`
}

type IssuanceConfig struct {
	Enabled bool `json:"enabled"`
	BlockReward int64 `json:"block_reward"`
	RewardAsset string `json:"reward_asset"`
	CoinbaseTo string `json:"coinbase_to"`
}

type ValidationConfig struct {
	ValidateForkState bool `json:"validate_fork_state"`
	MaxReplayBlocks uint64 `json:"max_replay_blocks"`
}

type PoSConfig struct {
	ValidatorSource string `json:"validator_source"`
}

type LimitsConfig struct {
	MaxBlockBytes int `json:"max_block_bytes"`
	MaxBlockTxs int `json:"max_block_txs"`
	MaxTxOutputs int `json:"max_tx_outputs"`
}

type NodeRoleConfig struct {
	Role string `json:"role"`      // gateway|validator|archive|observer|developer|miner
	ReadOnly bool `json:"read_only"`
	AllowPublicSubmit bool `json:"allow_public_submit"`
}

type StateConfig struct {
	SnapshotInterval uint64 `json:"snapshot_interval"`
	SnapshotDir string `json:"snapshot_dir"`
	SnapshotFormat string `json:"snapshot_format"`
}

type GossipConfig struct {
	Enabled bool `json:"enabled"`
	Peers []string `json:"peers"`
	TimeoutSeconds int `json:"timeout_seconds"`
	MaxHops int `json:"max_hops"`
}

type P2PConfig struct {
	Enabled bool `json:"enabled"`
	Bind string `json:"bind"`
	Port int `json:"port"`
	Allowlist []string `json:"allowlist"`
}

type FeesConfig struct {
	GasAsset string `json:"gas_asset"`
	BaseFeeMin int64 `json:"base_fee_min"`
	PerByte int64 `json:"per_byte"`
	OTAPMultiplier int64 `json:"otap_multiplier"`
	Mode string `json:"mode"` // NORMAL/DEFENSE/CONTAINMENT
}

type Config struct {
	Node struct {
		Name        string `json:"name"`
		RuntimeRoot string `json:"runtime_root"`
		LogFile     string `json:"log_file"`
	} `json:"node"`

	Storage struct {
		LevelDBPath string `json:"leveldb_path"`
	} `json:"storage"`

	Keyvault struct {
		Path   string `json:"path"`
		KEKEnv string `json:"kek_env"`
		RotationDays int `json:"rotation_days"`
	} `json:"keyvault"`

	RPC struct {
		Bind string `json:"bind"`
		Port int    `json:"port"`
	} `json:"rpc"`

	Chain struct {
		GenesisFile string `json:"genesis_file"`
	} `json:"chain"`

	Finality FinalityConfig `json:"finality"`
	Sync     SyncConfig     `json:"sync"`
	Fees     FeesConfig     `json:"fees"`
	PoP      PoPConfig      `json:"pop"`
	Mempool  MempoolConfig  `json:"mempool"`
	Issuance IssuanceConfig `json:"issuance"`
	P2P      P2PConfig      `json:"p2p"`
	Gossip   GossipConfig   `json:"gossip"`
	State    StateConfig    `json:"state"`
	Node     NodeRoleConfig `json:"node"`
	PoS      PoSConfig
	Limits   LimitsConfig   `json:"limits"`
	Validation ValidationConfig `json:"validation"`
}

func (c *Config) Validate() error {
	if c.Storage.LevelDBPath == "" { return errors.New("storage.leveldb_path required") }
	if c.Keyvault.Path == "" { return errors.New("keyvault.path required") }
	if c.Keyvault.KEKEnv == "" { c.Keyvault.KEKEnv = "RESERVEOS_KEYMASTER" }
	if c.Keyvault.RotationDays == 0 { c.Keyvault.RotationDays = 5 }
	if c.RPC.Port == 0 { return errors.New("rpc.port required") }
	if c.RPC.Bind == "" { c.RPC.Bind = "127.0.0.1" }
	if c.Chain.GenesisFile == "" { return errors.New("chain.genesis_file required") }

	if c.Finality.CheckpointInterval == 0 { c.Finality.CheckpointInterval = 20 }
	if c.Finality.ThresholdNum == 0 { c.Finality.ThresholdNum = 2 }
	if c.Finality.ThresholdDen == 0 { c.Finality.ThresholdDen = 3 }
	if c.Finality.GossipIntervalSeconds <= 0 { c.Finality.GossipIntervalSeconds = 3 }

	if c.Sync.IntervalSeconds <= 0 { c.Sync.IntervalSeconds = 3 }
	if c.Sync.MaxBlocksPerRound <= 0 { c.Sync.MaxBlocksPerRound = 50 }

	if c.Fees.GasAsset == "" { c.Fees.GasAsset = "USDR" }
	if c.Fees.BaseFeeMin <= 0 { c.Fees.BaseFeeMin = 100 }
	if c.Fees.PerByte <= 0 { c.Fees.PerByte = 2 }
	if c.Fees.OTAPMultiplier <= 0 { c.Fees.OTAPMultiplier = 10 }
	if c.Fees.Mode == "" { c.Fees.Mode = "NORMAL" }
	if c.PoP.EpochBlocks == 0 { c.PoP.EpochBlocks = 50 }
	if c.PoP.MinPayout == 0 { c.PoP.MinPayout = 100 }

	if c.Mempool.MaxTxs <= 0 { c.Mempool.MaxTxs = 5000 }
	if c.Mempool.MaxBytes <= 0 { c.Mempool.MaxBytes = 8_000_000 }
	if c.Mempool.TxTTLSeconds <= 0 { c.Mempool.TxTTLSeconds = 1800 }
	if c.Mempool.MaxNonceGap == 0 { c.Mempool.MaxNonceGap = 50 }
	if c.Mempool.RBFMinFeeBump <= 0 { c.Mempool.RBFMinFeeBump = 1.10 }

	if c.Issuance.RewardAsset == "" { c.Issuance.RewardAsset = "USDR" }
	if c.Issuance.BlockReward == 0 { c.Issuance.BlockReward = 5000 }
	if c.Issuance.CoinbaseTo == "" { c.Issuance.CoinbaseTo = "miner:local" }
	if c.P2P.Bind == "" { c.P2P.Bind = "0.0.0.0" }
	if c.P2P.Port == 0 { c.P2P.Port = 18444 }
	if c.Gossip.TimeoutSeconds <= 0 { c.Gossip.TimeoutSeconds = 3 }
	if c.Gossip.MaxHops <= 0 { c.Gossip.MaxHops = 1 }
	if c.State.SnapshotInterval == 0 { c.State.SnapshotInterval = 200 }
	if c.State.SnapshotDir == "" { c.State.SnapshotDir = "runtime/snapshots" }
	if c.State.SnapshotFormat == "" { c.State.SnapshotFormat = "json" }
	if c.Node.Role == "" { c.Node.Role = "developer" }
	if c.Limits.MaxBlockBytes == 0 { c.Limits.MaxBlockBytes = 1_000_000 }
	if c.Limits.MaxBlockTxs == 0 { c.Limits.MaxBlockTxs = 5000 }
	if c.PoS.ValidatorSource == "" { c.PoS.ValidatorSource = "registry" }
	if c.Limits.MaxTxOutputs == 0 { c.Limits.MaxTxOutputs = 256 }
	if c.Validation.MaxReplayBlocks == 0 { c.Validation.MaxReplayBlocks = 2000 }





	return nil
}
