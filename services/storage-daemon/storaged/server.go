package storaged

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Config struct {
	RPC struct {
		Bind string `json:"bind"`
		Port int    `json:"port"`
	} `json:"rpc"`
	DB struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
		KeyRef string `json:"key_ref"`
	} `json:"db"`
}

type Server struct {
	cfg Config
	db  *sql.DB
	mux *http.ServeMux
}

func New(cfgPath string) (*Server, error) {
	var cfg Config
	b, err := os.ReadFile(cfgPath)
	if err != nil { return nil, err }
	if err := json.Unmarshal(b, &cfg); err != nil { return nil, err }
	if cfg.RPC.Bind == "" { cfg.RPC.Bind = "127.0.0.1" }
	if cfg.RPC.Port == 0 { return nil, errors.New("rpc.port required") }
	if cfg.DB.Path == "" { return nil, errors.New("db.path required") }
	if cfg.DB.Mode == "" { cfg.DB.Mode = "sqlite" }

	if err := os.MkdirAll(filepath.Dir(cfg.DB.Path), 0700); err != nil { return nil, err }
	db, err := sql.Open("sqlite3", cfg.DB.Path)
	if err != nil { return nil, err }
	db.SetMaxOpenConns(1); db.SetMaxIdleConns(1)

	if err := migrate(db); err != nil { return nil, err }

	s := &Server{cfg: cfg, db: db, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

func (s *Server) Run() error {
	addr := s.cfg.RPC.Bind + ":" + strconv.Itoa(s.cfg.RPC.Port)
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	s.mux.HandleFunc("/alloc_slot", s.handleAllocSlot)
	s.mux.HandleFunc("/mark_confirmed", s.handleMarkConfirmed)
	s.mux.HandleFunc("/slot_set_hash", s.handleSlotSetHash) // kept for legacy compatibility
}

type allocSlotReq struct {
	AccountID *string `json:"account_id"`
	Purpose string `json:"purpose"`
	ExpiresAt *int64 `json:"expires_at"`
}

type allocSlotResp struct {
	SlotID int64 `json:"slot_id"`
	DerivationIndex uint64 `json:"derivation_index"`
	ExpiresAt *int64 `json:"expires_at"`
}

func (s *Server) handleAllocSlot(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { w.WriteHeader(405); return }
	var req allocSlotReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { w.WriteHeader(400); return }
	if req.Purpose == "" { w.WriteHeader(400); w.Write([]byte(`{"error":"missing purpose"}`)); return }

	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil { w.WriteHeader(500); return }
	defer tx.Rollback()

	counterName := "addr_index:"
	if req.AccountID != nil { counterName += *req.AccountID } else { counterName += "default" }
	counterName += ":" + req.Purpose

	var cur int64
	err = tx.QueryRow(`SELECT value FROM counters WHERE name=?`, counterName).Scan(&cur)
	if err == sql.ErrNoRows {
		cur = 0
		if _, err := tx.Exec(`INSERT INTO counters(name,value) VALUES(?,?)`, counterName, cur); err != nil { w.WriteHeader(500); return }
	} else if err != nil {
		w.WriteHeader(500); return
	}

	deriv := uint64(cur)
	if _, err := tx.Exec(`UPDATE counters SET value=value+1 WHERE name=?`, counterName); err != nil { w.WriteHeader(500); return }

	res, err := tx.Exec(`INSERT INTO address_slots(account_id,purpose,derivation_index,created_at,expires_at,status) VALUES(?,?,?,?,?,'allocated')`,
		req.AccountID, req.Purpose, int64(deriv), now, req.ExpiresAt)
	if err != nil { w.WriteHeader(500); return }
	slotID, _ := res.LastInsertId()

	if err := tx.Commit(); err != nil { w.WriteHeader(500); return }

	w.Header().Set("Content-Type","application/json")
	_ = json.NewEncoder(w).Encode(allocSlotResp{SlotID: slotID, DerivationIndex: deriv, ExpiresAt: req.ExpiresAt})
}

type markConfirmedReq struct {
	SlotID int64 `json:"slot_id"`
	Height int64 `json:"height"`
	TxID string `json:"txid"`
	ConfirmedAt int64 `json:"confirmed_at"`
}

func (s *Server) handleMarkConfirmed(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { w.WriteHeader(405); return }
	var req markConfirmedReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { w.WriteHeader(400); return }
	_, err := s.db.Exec(`UPDATE address_slots SET status='confirmed', confirmed_height=?, confirmed_txid=?, confirmed_at=? WHERE slot_id=?`,
		req.Height, req.TxID, req.ConfirmedAt, req.SlotID)
	if err != nil { w.WriteHeader(500); return }
	w.Write([]byte(`{"ok":true}`))
}

type slotSetHashReq struct {
	SlotID int64 `json:"slot_id"`
	AddressHash string `json:"address_hash"`
}

func (s *Server) handleSlotSetHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { w.WriteHeader(405); return }
	var req slotSetHashReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { w.WriteHeader(400); return }
	hb, err := hex.DecodeString(req.AddressHash)
	if err != nil || len(hb) != 32 { w.WriteHeader(400); return }
	_, err = s.db.Exec(`UPDATE address_slots SET address_hash=? WHERE slot_id=?`, hb, req.SlotID)
	if err != nil { w.WriteHeader(500); return }
	w.Write([]byte(`{"ok":true}`))
}
