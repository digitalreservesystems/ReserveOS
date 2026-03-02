package walletd

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"reserveos/core/chain"
	"reserveos/core/crypto/otap"
	"reserveos/core/crypto/schnorr"
	"filippo.io/edwards25519"
	"reserveos/internal/reservekeyvault"
)

const OTAPContext = "default" // keep fixed; slot_id goes in encrypted memo

type Config struct {
	RPC struct {
		Bind string `json:"bind"`
		Port int    `json:"port"`
	} `json:"rpc"`
	Storage struct {
		BaseURL string `json:"base_url"`
	} `json:"storage"`
	Node struct {
		BaseURL string `json:"base_url"`
	} `json:"node"`
	Polling struct {
		Enabled         bool `json:"enabled"`
		IntervalSeconds int  `json:"interval_seconds"`
	} `json:"polling"`
	Keyvault struct {
		Path   string `json:"path"`
		KEKEnv string `json:"kek_env"`
	} `json:"keyvault"`
}

type Server struct {
	acct *WalletAccount

	cfg     Config
	mux     *http.ServeMux
	store   *StorageClient
	node    *NodeClient
	keys    *OTAPKeys
	chainID string
}

func New(cfgPath string) (*Server, error) {
	var cfg Config
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if cfg.RPC.Port == 0 {
		return nil, errors.New("rpc.port required")
	}
	if cfg.RPC.Bind == "" {
		cfg.RPC.Bind = "127.0.0.1"
	}
	if cfg.Storage.BaseURL == "" {
		return nil, errors.New("storage.base_url required")
	}
	if cfg.Node.BaseURL == "" {
		return nil, errors.New("node.base_url required")
	}
	if cfg.Polling.IntervalSeconds <= 0 {
		cfg.Polling.IntervalSeconds = 2
	}
	if cfg.Keyvault.Path == "" {
		cfg.Keyvault.Path = "runtime/keyvault/keyvault.enc"
	}
	if cfg.Keyvault.KEKEnv == "" {
		cfg.Keyvault.KEKEnv = "RESERVEOS_KEYMASTER"
	}

	kv, err := reservekeyvault.OpenOrInit(reservekeyvault.OpenOptions{Path: cfg.Keyvault.Path, KEKEnv: cfg.Keyvault.KEKEnv, RotationDays: 5})
	if err != nil {
		return nil, err
	}
	keys, err := LoadOrGenOTAPKeys(kv)
	if err != nil { return nil, err }
	acct, err := LoadOrGenWalletAccount(kv)
	if err != nil { return nil, err }

	s := &Server{
		cfg:     cfg,
		mux:     http.NewServeMux(),
		store:   NewStorageClient(cfg.Storage.BaseURL),
		node:    NewNodeClient(cfg.Node.BaseURL),
		keys:    keys,
		acct:    acct,
		chainID: "",
	}
	s.routes()
	return s, nil
}

func (s *Server) Run() error {
	if s.cfg.Polling.Enabled {
		go s.pollLoop()
	}
	addr := s.cfg.RPC.Bind + ":" + strconv.Itoa(s.cfg.RPC.Port)
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	s.mux.HandleFunc("/otap/keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"S_pub": s.keys.Registry.ScanPub,
			"V_pub": s.keys.Registry.SpendPub,
		})
	})

	s.mux.HandleFunc("/otap/request", s.handleOTAPRequest)
	s.mux.HandleFunc("/otap/build_tx", s.handleOTAPBuildTx)
}

type otapRequestReq struct {
	Purpose   string  `json:"purpose"`
	ExpiresIn int64   `json:"expires_in"`
	AccountID *string `json:"account_id"`
}

type otapRequestResp struct {
	SlotID    int64  `json:"slot_id"`
	ExpiresAt *int64 `json:"expires_at"`
	S_pub     string `json:"S_pub"`
	V_pub     string `json:"V_pub"`
}

func (s *Server) handleOTAPRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var req otapRequestReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(400)
		return
	}
	if req.Purpose == "" {
		req.Purpose = "receive"
	}

	var expiresAt *int64
	if req.ExpiresIn > 0 {
		v := time.Now().Unix() + req.ExpiresIn
		expiresAt = &v
	}

	slot, err := s.store.AllocSlot(AllocSlotReq{AccountID: req.AccountID, Purpose: req.Purpose, ExpiresAt: expiresAt})
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"alloc_failed"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(otapRequestResp{
		SlotID:    slot.SlotID,
		ExpiresAt: slot.ExpiresAt,
		S_pub:     s.keys.Registry.ScanPub,
		V_pub:     s.keys.Registry.SpendPub,
	})
}

type otapBuildReq struct {
	Recipient otap.RegistryKeys `json:"recipient"`
	Amount    int64             `json:"amount"`
	Asset     string            `json:"asset"`
	SlotID    int64             `json:"slot_id"`
	Note      string            `json:"note"`
}

func (s *Server) handleOTAPBuildTx(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var req otapBuildReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(400)
		return
	}
	if req.Amount <= 0 {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"bad_amount"}`))
		return
	}
	if req.Asset == "" {
		req.Asset = "USDR"
	}
	if req.Recipient.ScanPub == "" || req.Recipient.SpendPub == "" {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"missing_recipient_keys"}`))
		return
	}

	chainID := s.chainID
	if chainID == "" {
		ci, err := s.node.ChainInfo()
		if err == nil {
			chainID = ci.ChainID
			s.chainID = chainID
		}
	}
	if chainID == "" {
		chainID = "reservechain-unknown"
	}

	out, K, err := otap.BuildOutput(chainID, OTAPContext, req.Recipient)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"otap_build_failed"}`))
		return
	}

	memoObj := map[string]any{"slot_id": req.SlotID}
	if req.Note != "" {
		memoObj["note"] = req.Note
	}
	memoPlain, _ := json.Marshal(memoObj)
	aad := []byte(out.P + out.R)
	encMemo, err := otap.EncryptMemo(K, memoPlain, aad)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"memo_encrypt_failed"}`))
		return
	}

	tx := map[string]any{
		"version": 1,
		"nonce":   1,
		"outputs": []any{
			map[string]any{
				"amount":      req.Amount,
				"asset":       req.Asset,
				"P":           out.P,
				"R":           out.R,
				"tag":         out.Tag,
				"enc_memo":    encMemo,
				"policy_bits": 1,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tx)
}

func (s *Server) pollLoop() {
	var last uint64 = 0
	interval := time.Duration(s.cfg.Polling.IntervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		ci, err := s.node.ChainInfo()
		if err != nil {
			continue
		}
		if s.chainID == "" {
			s.chainID = ci.ChainID
		}

		if last == 0 {
			last = ci.Height
			continue
		}
		if ci.Height <= last {
			continue
		}

		for h := last + 1; h <= ci.Height; h++ {
			br, err := s.node.BlockByHeight(h)
			if err != nil {
				break
			}

			for _, tx := range br.Block.Txs {
				for _, o := range tx.Outputs {
					if o.P == "" || o.R == "" {
						continue
					}
					matched, K, err := otap.Detect(s.chainID, OTAPContext, otap.OTAPOutput{P: o.P, R: o.R, Tag: o.Tag}, s.keys.Priv.ScanPriv, s.keys.Registry.SpendPub)
					if err != nil || !matched {
						continue
					}
					if o.EncMemo == "" {
						continue
					}
					aad := []byte(o.P + o.R)
					plain, err := otap.DecryptMemo(K, o.EncMemo, aad)
					if err != nil {
						continue
					}
					var memo map[string]any
					if err := json.Unmarshal(plain, &memo); err != nil {
						continue
					}
					slotAny, ok := memo["slot_id"]
					if !ok {
						continue
					}
					slotID := int64(0)
					switch v := slotAny.(type) {
					case float64:
						slotID = int64(v)
					case int64:
						slotID = v
					}
					if slotID <= 0 {
						continue
					}
					_ = s.store.MarkConfirmed(slotID, int64(h), "txid_todo", time.Now().Unix())
					// Auto-build and submit OTAP claim tx (v1)
					fee := int64(1000)

					// Build typed tx so SigningBytes match node rules
					tx := &chain.Tx{
						Type:    "otap_claim",
						From:    s.acct.PubHex,
						PubKey:  s.acct.PubHex,
						Nonce:   0, // filled from node state
						Fee:     fee,
						GasAsset:"USDR",
						Outputs: []chain.TxOutput{},
						OTAPClaim: &chain.OTAPClaim{
							P: o.P,
							R: o.R,
							Tag: o.Tag,
							To: s.acct.PubHex,
							Amount: o.Amount,
						},
					}

					// Fill nonce from chain state
					if si, err := s.node.StateInfo(s.acct.PubHex); err == nil {
						tx.Nonce = si.Nonce + 1
					} else {
						// can't determine nonce, skip this round
						continue
					}

					// Derive one-time private scalar x = v + k' and sign claim proof over tx.SigningBytes()
					matched2, _, kSc, err := otap.DetectWithK(s.chainID, OTAPContext, otap.OTAPOutput{P: o.P, R: o.R, Tag: o.Tag}, s.keys.Priv.ScanPriv, s.keys.Registry.SpendPub)
					if err == nil && matched2 {
						x, err := otap.OneTimePriv(s.keys.Priv.SpendPriv, kSc)
						if err == nil {
							Ppoint, err := otap.DecodePointHex(o.P)
							if err == nil {
								cs, _ := schnorr.Sign(x, Ppoint, tx.SigningBytes())
								tx.OTAPClaim.ClaimSigHex = cs
								// Account signature over same signing bytes
								asig, _ := s.acct.Sign(tx.SigningBytes())
								tx.SigHex = asig
								_, _ = s.node.SubmitTx(tx)
							}
						}
					}

						}
					}

				}
			}
			last = h
		}
	}
}
