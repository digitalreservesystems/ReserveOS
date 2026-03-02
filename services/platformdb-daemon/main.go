package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"database/sql"

	"reserveos/internal/reserveconfig"
	"reserveos/internal/reservekeyvault"
)

type Config struct {
	RPC struct{
		Bind string `json:"bind"`
		Port int `json:"port"`
	} `json:"rpc"`
	DB struct{
		Path string `json:"path"`
		Mode string `json:"mode"` // "sqlite" or "sqlcipher" (best-effort)
		KeyvaultPath string `json:"keyvault_path"`
		KeyvaultEnv string `json:"keyvault_env"`
	} `json:"db"`
}

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Println("usage: platformdb-daemon <config.json>")
		os.Exit(2)
	}
	cfgPath := flag.Arg(0)
	var cfg Config
	if err := reserveconfig.LoadJSON(cfgPath, &cfg); err != nil { log.Fatal(err) }
	if cfg.RPC.Bind == "" { cfg.RPC.Bind = "127.0.0.1" }
	if cfg.RPC.Port == 0 { cfg.RPC.Port = 9030 }
	if cfg.DB.Path == "" { cfg.DB.Path = "runtime/platformdb/platform.db" }
	if cfg.DB.Mode == "" { cfg.DB.Mode = "sqlite" }
	if cfg.DB.KeyvaultPath == "" { cfg.DB.KeyvaultPath = "runtime/keyvault/keyvault.enc" }
	if cfg.DB.KeyvaultEnv == "" { cfg.DB.KeyvaultEnv = "RESERVEOS_KEYMASTER" }

	_ = os.MkdirAll("runtime/platformdb", 0700)

	kv, err := reservekeyvault.OpenOrInit(reservekeyvault.OpenOptions{Path: cfg.DB.KeyvaultPath, KEKEnv: cfg.DB.KeyvaultEnv, RotationDays: 5})
	if err != nil { log.Fatal(err) }
	// DB key is stored in keyvault; on first run, create one
	key, ok := kv.GetString("platformdb.sqlcipher_key")
	if !ok || key == "" {
		key = fmt.Sprintf("devkey-%d", os.Getpid())
		_ = kv.SetString("platformdb.sqlcipher_key", key)
	}

	dsn := cfg.DB.Path
	if cfg.DB.Mode == "sqlcipher" {
		// best-effort: if linked with SQLCipher, this activates encryption
		dsn = fmt.Sprintf("file:%s?_pragma_key=%s&_pragma_cipher_page_size=4096", cfg.DB.Path, key)
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil { log.Fatal(err) }
	defer db.Close()

	_, _ = db.Exec(`PRAGMA journal_mode=WAL;`)
	_, _ = db.Exec(`PRAGMA busy_timeout=5000;`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS kv (k TEXT PRIMARY KEY, v TEXT, updated_unix INTEGER)`)

	h := http.NewServeMux()
	// token for writes
	token, okTok := kv.GetString("platformdb.token")
	if !okTok || token == "" { token = fmt.Sprintf("tok-%d", time.Now().Unix()); _ = kv.SetString("platformdb.token", token) }
	h.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request){ w.Write([]byte("ok")) })
	h.HandleFunc("/kv/get", func(w http.ResponseWriter, r *http.Request){
		k := r.URL.Query().Get("k")
		row := db.QueryRow(`SELECT v FROM kv WHERE k=?`, k)
		var v string
		err := row.Scan(&v)
		w.Header().Set("Content-Type","application/json")
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "k": k, "v": v})
	})
	h.HandleFunc("/kv/set", func(w http.ResponseWriter, r *http.Request){
		if r.Method != "POST" { w.WriteHeader(405); return }
		if r.Header.Get("X-Platform-Token") != token { w.WriteHeader(401); w.Write([]byte(`{"error":"unauthorized"}`)); return }
		var body struct{ K string `json:"k"`; V string `json:"v"` }
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = db.Exec(`INSERT INTO kv(k,v,updated_unix) VALUES(?,?,strftime('%s','now')) ON CONFLICT(k) DO UPDATE SET v=excluded.v, updated_unix=excluded.updated_unix`, body.K, body.V)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	addr := fmt.Sprintf("%s:%d", cfg.RPC.Bind, cfg.RPC.Port)
	log.Println("platformdb-daemon listening on", addr, "mode", cfg.DB.Mode)
	log.Fatal(http.ListenAndServe(addr, h))
}
