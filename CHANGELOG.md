# ReserveOS Changelog

## v1.0.0 — 2026-03-01
### Added
- `VERSION` (project version marker)
- `CHANGELOG.md` (this file)
- OTAP claim proof verification via `otap_claim.claim_sig`:
  - `core/chain/tx.go` (added `OTAPClaim.ClaimSigHex`, ensured signing bytes exclude claim sig)
  - `core/node/node.go` (enforce and verify claim signature against `P`)
  - `core/crypto/sig/ed25519.go` (added helper)

### Modified
- `README.md` (documented OTAP claim proof requirements)

### Notes
- ZIP naming convention starts now: `ReserveOS-V<version>.zip`


## v1.0.1 — 2026-03-01
### Added
- Schnorr25519 claim proof verification:
  - `core/crypto/schnorr/schnorr25519.go`
  - `core/crypto/otap/otap.go` (added `DetectWithK`, `OneTimePriv`)
- Wallet auto-claim scaffold:
  - `services/wallet-daemon/walletd/account_keys.go` (wallet ed25519 key stored in keyvault)
  - `services/wallet-daemon/walletd/node_client.go` (tx submit helper)
  - `services/wallet-daemon/walletd/server.go` (auto-claim submission after detection)

### Modified
- `core/node/node.go` (OTAP claim proofs now verified with Schnorr over `P`; claim fee deducted from OTAP bucket; claim msg binding)
- `core/node/state_apply.go` (otap_claim now deducts fee from OTAP bucket)
- `core/chain/tx.go` (updated claim_sig semantics)
- `README.md` (updated claim requirements and wallet auto-claim notes)

### Notes
- Claim proof uses message binding `claim|P|from|amount|fee` in this version (simplified deterministic signing target).


## v1.0.2 — 2026-03-01
### Modified
- Canonical OTAP claim signing:
  - `core/node/node.go` now verifies both account signature and claim Schnorr signature against `tx.SigningBytes()`
  - `services/wallet-daemon/walletd/server.go` now constructs a typed `chain.Tx` and signs canonical `SigningBytes()` for both signatures
- `core/crypto/otap/otap.go` added exported helper `DecodePointHex` for wallet services

### Notes
- Removed the v1.0.1 simplified string message binding; signatures are now stable/canonical via chain tx encoding.


## v1.0.3 — 2026-03-01
### Added
- State introspection endpoints:
  - `core/node/node.go`: `GET /state/nonce`, `GET /state/info`
- Wallet nonce-safe auto-claim:
  - `services/wallet-daemon/walletd/node_client.go`: `StateInfo()`
  - `services/wallet-daemon/walletd/server.go`: auto-claim now queries nonce and uses `nonce+1`

### Modified
- `README.md` (documented new state endpoints)


## v1.0.4 — 2026-03-01
### Added
- Canonical deterministic transaction encoding:
  - `core/chain/codec.go` (`CanonicalTxBytes`)

### Modified
- `core/chain/tx.go`
  - `SigningBytes()` now uses canonical encoding
  - `ID()` now hashes canonical signing bytes (signatures excluded)

### Notes
- All signature verification now implicitly benefits from canonical bytes because node and wallet use `SigningBytes()`.


## v1.0.5 — 2026-03-01
### Added
- Mempool correctness + RBF + fee-rate priority:
  - `core/node/mempool.go`
- Node config: `mempool` section in `config/node/node.json`

### Modified
- `core/node/node.go` (uses new mempool and fee-rate draining)
- `core/node/config.go` (added `MempoolConfig`)



## v1.0.6 — 2026-03-01
### Added
- `config/node/node.json (issuance section)`
- `core/node/config.go (IssuanceConfig defaults)`
### Modified
- `core/node/node.go (coinbase tx on mining; forbid external coinbase submits)`
- `README.md (issuance note)`
### Notes
- Coinbase tx is internal to miner; outputs are credited by the existing state apply logic.


## v1.0.7 — 2026-03-01
### Added
- `core/node/node.go (isLocalValidator helper)`
- `core/node/node.go (GET /finality/checkpoints stub)`
### Modified
- `core/node/node.go (auto-vote only when local validator is in set)`
- `README.md (finality note)`
### Notes
- /finality/checkpoints is a stub list of checkpoint heights in this version; next update can add proper checkpoint retrieval.


## v1.0.8 — 2026-03-01
### Added
- `internal/reservestorage/pop_scores.go`
- `core/node/node.go (POST /pop/heartbeat, GET /pop/score)`
### Modified
- `core/economics/pop/pop.go (payout weights by PoP score, reset scores after payout)`
- `README.md (PoP scoring docs)`


## v1.0.9 — 2026-03-01
### Added
- `internal/reservekeyvault/keyvault.go (rotation meta + key_id derivation)`
- `config/node/node.json (keyvault.rotation_days)`
### Modified
- `core/node/config.go (keyvault.rotation_days)`
- `core/node/node.go (pass RotationDays)`
- `services/wallet-daemon/walletd/server.go (RotationDays=5)`
- `README.md (rotation note)`
### Notes
- Rotation derives AES key as SHA256(KEK|key_id) and updates key_id when age exceeds rotation_days.


## v1.0.10 — 2026-03-01
### Added
- `services/platformdb-daemon/main.go`
- `config/services/platformdb-daemon/service.json`
### Notes
- SQLCipher support depends on how sqlite3 is compiled; service uses _pragma_key DSN when mode=sqlcipher.


## v1.0.11 — 2026-03-01
### Added
- `internal/reservep2p/p2p.go`
- `internal/reservep2p/dial.go`
- `config/node/node.json (p2p section)`
### Modified
- `core/node/config.go (P2PConfig)`
- `core/node/node.go (start/stop p2p listener)`
- `README.md (p2p note)`
### Notes
- Handshake is signed hello only; session encryption is a future upgrade.


## v1.0.12 — 2026-03-01
### Added
- Mempool introspection:
  - `core/node/node.go` (`GET /mempool/info`)
  - `core/node/mempool.go` (`Info()`)

### Modified
- `core/node/mempool.go`
  - `Add(db, tx)` now enforces `allow_future_nonces` and `max_nonce_gap` at admission time
- `core/node/node.go` (passes LevelDB handle into mempool admission)
- `README.md` (documented /mempool/info)


## v1.0.13 — 2026-03-02
### Added
- `services/initialize-daemon/main.go` (gateway orchestrator + reverse proxy + flat webfs builder)
- `internal/reservewebfs/webfs.go` (flat runtime/web builder)
- `config/gateway/gateway.json` (single-port gateway preset)
- UI placeholders:
  - `ui/marketing/index.html`
  - `ui/portal/index.html`
  - `ui/wallet/index.html`

### Modified
- `README.md` (gateway run instructions)

### Notes
- Gateway mode uses a single HTTP port with path-based routing:
  - `/` → marketing
  - `/portal` → portal
  - `/wallet` → wallet
  - `/api/*` → reverse proxy to core node RPC


## v1.0.14 — 2026-03-02
### Added
- `internal/reservestorage/pos_validators.go`
- `internal/reservestorage/checkpoints.go`
- `core/node/node.go` (`/pos/register`, `/pos/validators`)
### Modified
- `core/node/node.go` (`/finality/checkpoints` returns stored checkpoints)
### Notes
- Validator set is dev-registrable in this version; slashing hooks are scaffolded for next iteration.


## v1.0.15 — 2026-03-02
### Added
- `internal/reservestorage/pop_events.go`
- `core/node/node.go` (`POST /pop/event`)
### Notes
- Typed PoP events update scores using multipliers: relay=5, storage=3, uptime=2.


## v1.0.16 — 2026-03-02
### Added
- `config/keyvault/presets.json`
### Notes
- Presets are configuration helpers for rotation cadence.


## v1.0.17 — 2026-03-02
### Modified
- `services/platformdb-daemon/main.go` (WAL, busy_timeout, token auth on /kv/set)
### Notes
- `/kv/set` now requires `X-Platform-Token` stored in keyvault as `platformdb.token`.


## v1.0.18 — 2026-03-02
### Added
- `internal/reservep2p/messages.go`
### Notes
- Message framing is ready for wiring into encrypted transport next.


## v1.0.19 — 2026-03-02
### Added
- `core/node/gossip.go`
- `core/node/node.go` (`/gossip/tx`, `/gossip/info`, `validateTxAdmission`)
- `core/node/config.go` (`GossipConfig`)
- `config/node/node.json` (`gossip`)
### Notes
- Gossip uses HTTP ports for now; can be migrated to encrypted P2P later.


## v1.0.20 — 2026-03-02
### Added
- Gateway API routing presets:
  - `config/gateway/gateway.json` adds `/api/node`, `/api/wallet`, `/api/platformdb`
- `services/initialize-daemon/main.go` adds `/api/healthz` and optional PlatformDB token injection via env

### Modified
- `README.md` (documented gateway API routing)

### Notes
- This keeps the single-port gateway model while cleanly separating upstream APIs by prefix.


## v1.0.21 — 2026-03-02
### Added
- `services/initialize-daemon/main.go` (`GET /api/network/status` aggregate endpoint)
### Modified
- `ui/marketing/index.html` (added link to network status)
- `README.md` (documented /api/network/status)


## v1.0.22 — 2026-03-02
### Added
- `core/node/gossip_block.go`
- `core/node/node.go (POST /gossip/block)`
### Modified
- `core/node/node.go (broadcast mined blocks)`
- `README.md`
### Notes
- This is HTTP-based gossip on the RPC port; encrypted TCP gossip can be wired later via reservep2p.


## v1.0.23 — 2026-03-02
### Added
- `internal/reservestorage/peers.go`
- `core/node/node.go (/peers/add, /peers/list)`
### Modified
- `core/node/gossip.go`
- `core/node/gossip_block.go`
- `README.md`
### Notes
- Peers are stored in LevelDB; gateway can manage peers without editing config files.


## v1.0.24 — 2026-03-02
### Added
- `config/gateway/gateway.json (tcp section)`
- `services/initialize-daemon/main.go (startTCPProxy)`
### Modified
- `services/initialize-daemon/main.go (TCP proxy start)`
- `README.md`
### Notes
- HTTP remains single-port; TCP proxy is separate listener for P2P traffic (expected for gateway nodes).


## v1.0.25 — 2026-03-02
### Added
- `internal/reservewebfs/webfs.go (hashed assets + placeholder rewriting)`
- `ui/marketing/app.css`
### Modified
- `ui/marketing/index.html`
- `README.md`
### Notes
- This keeps runtime/web flat while still supporting cache-busting assets.


## v1.0.26 — 2026-03-02
### Added
- `scripts/build_gateway.sh`
- `scripts/build_gateway.bat`
- `config/gateway/presets.json`
### Modified
- `README.md`
### Notes
- Preset uses built binaries under build/ReserveOS; initialize-daemon can be pointed at gateway.json or a preset-derived config.


## v1.0.27 — 2026-03-02
### Added
- `services/initialize-daemon/main.go (rate limiter)`
### Modified
- `services/initialize-daemon/main.go (wrap handler)`
- `README.md`
### Notes
- In-memory limiter only; production version should use shared store and proper IP parsing behind proxies.


## v1.0.28 — 2026-03-02
### Added
- `services/initialize-daemon/main.go`
  - `GET /api/network/peers`
  - `POST /api/network/peers/add`
### Modified
- `ui/marketing/index.html` (added peers link)
- `README.md` (documented peer control)


## v1.0.29 — 2026-03-02
### Added
- `core/node/block_validate.go` (block validity rules + tip-state simulation)

### Modified
- `core/node/node.go`
  - `SubmitBlock` now validates blocks (coinbase rules, tx signatures; state simulation when extending tip)
  - PoS `tryFinalizeHeight` now uses validator registry weights when present and stores checkpoints on finalize
- `core/node/finality_gossip.go` (vote ingest now respects validator registry and ignores slashed validators)
- `README.md` (documented node hardening)

### Notes
- Fork blocks are stored after basic validation; full balance/nonce simulation is only enforced when the block extends the current selected tip.


## v1.0.30 — 2026-03-02
### Added
- `core/node/block_assemble.go` (simulated-state tx filtering for block assembly)
- `core/node/mempool.go` (`EligibleSnapshot`, `QueueSnapshot`)
- `core/node/node.go` (`GET /mempool/eligible`, `GET /mempool/queue`)

### Modified
- `core/node/node.go` (mining filters drained txs through simulated state)
- `README.md` (documented new mempool endpoints)


## v1.0.31 — 2026-03-02
### Added
- `internal/reservep2p/session.go` (dial/serve sessions + AES-GCM framed send/receive)
- `core/node/p2p_gossip.go` (node wiring for encrypted tx/block gossip)
- `core/node/node.go` (`GET /p2p/sessions`)

### Modified
- `internal/reservep2p/p2p.go` (session management, handler callbacks, dialing support)
- `core/node/node.go` (broadcast tx/block over P2P in addition to HTTP gossip)
- `README.md` (documented encrypted P2P gossip)

### Notes
- P2P peer dialing uses hostnames from `gossip.peers` and assumes TCP port 18444 (gateway proxies this).


## v1.0.32 — 2026-03-02
### Modified
- `internal/reservep2p/messages.go` (added id/hash fields; new message types)
- `core/node/p2p_gossip.go` (inv/get flow for txs and blocks + per-peer rate limiter)
- `README.md` (documented inv/get gossip)

### Notes
- P2P broadcasts now send inventories; full bodies are sent only on request.


## v1.0.33 — 2026-03-02
### Added
- `internal/reservestorage/txindex.go` (txid -> tx storage)
- `core/node/node.go` (`GET /tx/get`)
### Modified
- `core/node/node.go` (index txs on submit)
- `core/node/state_apply.go` (index txs when applying blocks)
- `core/node/p2p_gossip.go` (get_tx now serves from tx index)
- `README.md` (tx index docs)


## v1.0.34 — 2026-03-02
### Added
- `internal/reservestorage/peerstats.go`
- `core/node/node.go` (`GET /p2p/peerstat`, `POST /p2p/ban`)

### Modified
- `core/node/p2p_gossip.go` (ban checks + peer scoring/penalties)
- `README.md` (documented peer scoring)

### Notes
- v1 scaffold for anti-abuse; future updates can add IP bans and allowlists.


## v1.0.35 — 2026-03-02
### Added
- `core/state/snapshot.go` (write/list/read/restore snapshots)
- `core/node/node.go` (`GET /state/snapshots`, `POST /state/snapshot/trigger`)
- `config/node/node.json` (`state.snapshot_interval`, `state.snapshot_dir`)

### Modified
- `core/node/state_apply.go` (rebuild state from latest snapshot + replay remaining blocks)
- `core/node/node.go` (periodic snapshot on mined blocks)
- `README.md` (snapshot docs)

### Notes
- Snapshot format is gzipped JSON (v1). Future optimizations can switch to a binary snapshot for speed/size.


## v1.0.36 — 2026-03-02
### Added
- `config/node/node.json` (`node.role`, `node.read_only`)
- `core/node/node.go` (`GET /node/role`)

### Modified
- `core/node/config.go` (added `NodeRoleConfig`)
- `core/node/node.go` (read-only enforcement for tx submit, mining, and checkpoint auto-signing)
- `README.md` (node roles docs)

### Notes
- `role:"observer"` automatically implies read-only.


## v1.0.37 — 2026-03-02
### Added
- `config/node/node.json` (`limits`)
- `core/node/limits_validate.go`
- `core/chain/block.go` (`SizeHint`)
### Modified
- `core/node/config.go`
- `core/node/node.go`


## v1.0.38 — 2026-03-02
### Added
- `core/node/node.go` (`GET /chain/headers`, `GET /chain/locator`)


## v1.0.39 — 2026-03-02
### Modified
- `internal/reservep2p/p2p.go` (added `SendTo`)
- `core/node/p2p_gossip.go` (targeted get replies)


## v1.0.40 — 2026-03-02
### Added
- `config/node/node.json` (`p2p.allowlist`)
### Modified
- `core/node/config.go`
- `core/node/p2p_gossip.go`


## v1.0.41 — 2026-03-02
### Added
- `internal/reservestorage/seen.go`
### Modified
- `core/node/p2p_gossip.go`


## v1.0.42 — 2026-03-02
### Notes
- Maintenance release.


## v1.0.43 — 2026-03-02
### Added
- `config/node/node.json` (`node.allow_public_submit`)
### Modified
- `core/node/config.go`
- `core/node/node.go`


## v1.0.44 — 2026-03-02
### Added
- `core/node/node.go` (`POST /rpc`)


## v1.0.45 — 2026-03-02
### Added
- `core/node/node.go` (`GET /metrics`)


## v1.0.46 — 2026-03-02
### Added
- `services/initialize-daemon/main.go` (`GET /api/network/metrics`)


## v1.0.47 — 2026-03-02
### Added
- `core/node/block_validate_fork.go` (snapshot+replay fork state validation)
- `config/node/node.json` (`validation`)
### Modified
- `core/node/config.go` (`ValidationConfig`)
- `core/node/node.go` (enforce fork state validation on main-parent forks)


## v1.0.48 — 2026-03-02
### Added
- `internal/reservestorage/finality_finalized.go` (store finalized height)
- `core/node/finality_weighted.go` (weighted vote tally + finalize)
- `core/node/node.go` (`GET /finality/finalized`)

### Modified
- `core/node/node.go` (reject votes <= finalized height; require validator registry membership; finalize on vote ingestion; include finalized height in /chain/info)
- `README.md` (finality docs)

### Notes
- Quorum rule: >= 2/3 of total non-slashed validator weight at the checkpoint height.


## v1.0.49 — 2026-03-02
### Added
- `internal/reservestorage/ancestors.go` (ancestor hash lookup)
- `core/node/node.go` (`GET /finality/status`, finalized guard on tip selection)

### Notes
- Fork-choice now rejects any candidate tip whose ancestor at `finalized_height` does not match the current main chain hash at that height.



## v1.0.50 — 2026-03-02
### Added
- `core/node/node.go` (`GET /finality/tally?height=` weighted tally per block hash)



## v1.0.51 — 2026-03-02
### Added
- `config/node/node.json` (`pos.validator_source`)
- `core/node/config.go` (`PoSConfig`)
- `core/node/node.go` (`GET /pos/validators/source`)



## v1.0.52 — 2026-03-02
### Modified
- `core/node/p2p_gossip.go` (P2P peer exchange: `get_peers`/`peers`)

### Notes
- Peer exchange is best-effort and still gated by allowlist/ban logic.



## v1.0.53 — 2026-03-02
### Modified
- `core/node/p2p_gossip.go` (P2P headers messages: `get_headers` -> `headers`)

### Notes
- This is a scaffold; the full sync engine wiring will come next.



## v1.0.54 — 2026-03-02
### Added
- `internal/reservestorage/mempool_persist.go` (persist mempool txs with TTL)

### Modified
- `core/node/node.go` (persist txs on submit; reload on start; delete when mined)

### Notes
- Persistence is best-effort and TTL-based (default 1 hour).



## v1.0.55 — 2026-03-02
### Modified
- `core/node/block_validate.go` (coinbase must cover reward + total fees)

### Notes
- v1 rule: coinbase outputs must be >= issuance reward + sum(tx.fee).



## v1.0.56 — 2026-03-02
### Added
- `config/node/node.json` (`state.snapshot_format`)
- `core/state/snapshot_bin.go` (gob+gzip snapshot functions)

### Modified
- `core/node/config.go` (StateConfig.SnapshotFormat)
- `core/node/node.go` (snapshot trigger honors format)

### Notes
- Binary snapshot writing is scaffolded; state export maps will be completed next (currently returns empty maps).



## v1.0.57 — 2026-03-02
### Modified
- `services/initialize-daemon/main.go` (real IP parsing, request size cap for /api, HTTP server timeouts)

### Notes
- IP parsing uses X-Forwarded-For first hop (sufficient for local reverse proxies).



## v1.0.58 — 2026-03-02
### Added
- `config/node/presets.json` (node role presets)
- `scripts/apply_preset.sh`
- `scripts/apply_preset.bat`

### Notes
- Presets only patch `config/node/node.json` node role flags (services presets live in gateway presets).



## v1.0.59 — 2026-03-02
### Added
- `core/state/snapshot_auto.go` (auto-detect snapshot reader)
- `core/node/node.go` (`GET /state/snapshot/info?path=...`)

### Modified
- `core/state/snapshot_bin.go` (implemented `ExportStateMaps`, real binary snapshots)
- `core/state/snapshot.go` (ListSnapshots includes `.gob.gz`)
- `core/node/state_apply.go` (uses `ReadSnapshotAuto`)



## v1.0.60 — 2026-03-02
### Added
- `core/node/node.go` (`GET /finality/anchor` height+hash for finalized anchor)

### Notes
- This enables peers/gateways to require anchor agreement before fast sync (full enforcement wired in sync manager next).



## v1.0.61 — 2026-03-02
### Added
- `core/node/sync_manager.go` (minimal headers-first sync requester + status)
- `core/node/node.go` (`GET /sync/status`)

### Modified
- `core/node/node.go` (starts sync manager)
- `core/node/p2p_gossip.go` (records last headers count)

### Notes
- This is an initial P2P headers-first loop; block fetching pipeline comes next.



## v1.0.62 — 2026-03-02
### Modified
- `core/node/sync_manager.go` (tracks in-flight blocks)
- `core/node/p2p_gossip.go` (clears in-flight on block receipt)

### Notes
- Full header parsing and selective block requesting will be wired next (this version adds the bookkeeping hooks).



## v1.0.63 — 2026-03-02
### Added
- `internal/reservestorage/mempool_list.go` (list persisted mempool items)
- `core/node/node.go` (`GET /mempool/persisted`, `POST /mempool/prune`)



## v1.0.64 — 2026-03-02
### Modified
- `internal/reservep2p/p2p.go` (1MB max encrypted frame)
- `core/node/p2p_gossip.go` (basic message size cap)

### Notes
- This is a first layer of P2P DoS protection; rate limits + bans already exist.



## v1.0.65 — 2026-03-02
### Modified
- `core/node/block_assemble.go` (deterministic ordering: fee desc, then txid)
- `core/node/node.go` (`GET /mine/template`)

### Notes
- Makes block templates stable across nodes given the same mempool/state.



## v1.0.66 — 2026-03-02
### Added
- `internal/reservestorage/fees_block.go` (store per-block fee sum)
- `core/node/node.go` (`GET /fees/reconcile?from=&to=`)

### Modified
- `core/node/state_apply.go` (records fee sums on block apply)



## v1.0.67 — 2026-03-02
### Added
- `core/node/node.go` (`GET /pos/slashing/status`)

### Modified
- `core/node/node.go` (slashes validator if it votes for unknown block hash)



## v1.0.68 — 2026-03-02
### Added
- `services/initialize-daemon/main.go` (`GET /api/gateway/status` health aggregation)

### Notes
- Uses simple upstream /healthz checks (HTTP). TCP/P2P port probing can be added next.



## v1.0.69 — 2026-03-02
### Modified
- `core/node/p2p_gossip.go` (typed headers + request missing blocks)



## v1.0.70 — 2026-03-02
### Modified
- `services/wallet-daemon/main.go` (GET /healthz)
- `services/platformdb-daemon/main.go` (GET /healthz)



## v1.0.71 — 2026-03-02
### Modified
- `services/initialize-daemon/main.go` (/api/gateway/status probes TCP 18444)



## v1.0.72 — 2026-03-02
### Modified
- `internal/reservestorage/mempool_persist.go` (PutMempoolTxWithOrigin)
- `core/node/p2p_gossip.go` (persist origin peer)



## v1.0.73 — 2026-03-02
### Modified
- `core/node/p2p_gossip.go` (PEX cap + only http/https peers)



## v1.0.74 — 2026-03-02
### Modified
- `core/node/node.go` (/finality/tally returns checkpoint_id)



## v1.0.75 — 2026-03-02
### Modified
- `core/node/sync_manager.go` (store last header hashes)
- `core/node/p2p_gossip.go` (record hashes)
- `core/node/node.go` (GET /sync/last_headers)



## v1.0.76 — 2026-03-02
### Modified
- `core/node/p2p_gossip.go` (per-type message size caps)



## v1.0.77 — 2026-03-02
### Added
- `internal/reservestorage/peerstats_list.go`
- `core/node/node.go` (GET /p2p/bans, POST /p2p/unban)



## v1.0.78 — 2026-03-02
### Added
- `scripts/build_profile.sh`
- `scripts/build_profile.bat`



## v1.0.79 — 2026-03-02
### Added
- `core/node/sync_manager.go` (pending queue + in-flight tracking + fetch worker)

### Modified
- `core/node/node.go` (starts sync manager; exposes /sync/status if missing)
- `core/node/p2p_gossip.go` (headers handler enqueues; block receipt clears in-flight)
- `README.md`



## v1.0.80 — 2026-03-02
### Added
- `config/node/node.json` (`sync.workers`, `sync.max_inflight_blocks`)
- `core/node/config.go` (`SyncConfig`)

### Modified
- `core/node/sync_manager.go` (multi-worker block fetch + inflight cap; sends get_anchor)
- `core/node/p2p_gossip.go` (anchor handshake: get_anchor/anchor; enforce anchor match for headers/blocks once finalized)
- `README.md`



## v1.0.81 — 2026-03-02
### Modified
- `core/node/sync_manager.go` (peer backoff + targeted block requests)
- `core/node/node.go` (sync peer selection; `GET /sync/peers`)
- `core/node/p2p_gossip.go` (marks peer failures on invalid blocks)
- `README.md`



## v1.0.82 — 2026-03-02
### Modified
- `core/node/p2p_gossip.go` (continuity guard for incoming headers; clears inflight using computed hash if needed)
- `core/node/node.go` (`POST /sync/reset` debug endpoint)
- `README.md`



## v1.0.83 — 2026-03-02
### Modified
- `core/node/sync_manager.go` (P2P get_headers now includes a locator)
- `core/node/p2p_gossip.go` (get_headers can use locator; validates header chain connectivity)
### Notes
- Headers responses are rejected if they don't connect to the current tip (stronger anti-eclipse safety).



## v1.0.84 — 2026-03-02
### Added
- `config/node/node.json` (`sync.strict_linear`)

### Modified
- `core/node/config.go` (SyncConfig.StrictLinear)
- `core/node/node.go` (when strict_linear enabled, tip only advances for direct tip-extension blocks)
- `README.md`



## v1.0.85 — 2026-03-02
### Added
- `internal/reservestorage/candidates.go` (candidate block index by height)

### Modified
- `core/node/node.go` (stages blocks as candidates; `TryAdvanceTip` advances tip linearly; `POST /sync/advance`)
- `README.md`



## v1.0.86 — 2026-03-02
### Added
- `internal/reservestorage/headers_frontier.go` (store header frontier height+hash)
- `core/node/node.go` (`GET /sync/frontier`)

### Modified
- `core/node/p2p_gossip.go` (updates frontier on headers receive)



## v1.0.87 — 2026-03-02
### Modified
- `core/node/sync_manager.go` (header requests now advance from stored frontier when available)



## v1.0.88 — 2026-03-02
### Modified
- `core/node/node.go` (GC candidate blocks below finalized height; `GET /sync/candidates?height=` debug)



## v1.0.89 — 2026-03-02
### Modified
- `core/node/sync_manager.go` (pause toggle)
- `core/node/node.go` (`POST /sync/pause`)



## v1.0.90 — 2026-03-02
### Added
- `internal/reservestorage/headers_index.go` (header index + header tip)

### Modified
- `core/node/p2p_gossip.go` (stores headers into index; updates header tip)
- `core/node/sync_manager.go` (deterministic missing-block enqueue from header tip)
- `core/node/node.go` (`GET /sync/header_tip`)
- `README.md`



## v1.0.91 — 2026-03-02
### Modified
- `core/node/node.go` (after linear tip advancement, triggers missing-block scheduler to continue sync)



## v1.0.92 — 2026-03-02
### Added
- `config/node/node.json` (`sync.stage_candidates`)
### Modified
- `core/node/config.go` (SyncConfig.StageCandidates)
- `core/node/node.go` (candidate staging gated by config; default enabled)



## v1.0.93 — 2026-03-02
### Added
- `core/node/node.go` (`GET /sync/missing` reports header_tip vs block_tip gap)



## v1.0.94 — 2026-03-02
### Added
- `internal/reservestorage/cumwork.go` (store cumulative work per header hash)

### Modified
- `core/node/p2p_gossip.go` (computes cumwork on headers ingest; updates best header tip by cumwork with finalized guard)
- `core/node/node.go` (`GET /sync/best_header_tip`)
- `README.md`



## v1.0.95 — 2026-03-02
### Added
- `internal/reservestorage/header_ancestors.go` (walk header chain to ancestor height)
- `core/node/node.go` (`GET /sync/plan`)

### Modified
- `core/node/sync_manager.go` (missing-block scheduling follows best header chain, not just height->hash)
- `README.md`



## v1.0.96 — 2026-03-02
### Modified
- `internal/reservestorage/headers_index.go` (fork-aware header height index; supports multiple hashes per height)



## v1.0.97 — 2026-03-02
### Added
- `core/node/node.go` (`GET /sync/headers_at_height?h=` lists header hashes stored at a height)



## v1.0.98 — 2026-03-02
### Notes
- Compatibility update after fork-aware header height index (no functional change if already chain-walk scheduling).



## v1.0.99 — 2026-03-02
### Added
- `config/node/node.json` (`sync.max_queue_fill`)
### Modified
- `core/node/config.go` (SyncConfig.MaxQueueFill)
- `core/node/sync_manager.go` (limits how many hashes are enqueued per scheduler tick)



## v1.0.100 — 2026-03-02
### Modified
- `core/node/p2p_gossip.go` (rejects P2P blocks whose header hash is unknown in header index)



## v1.0.101 — 2026-03-02
### Added
- `internal/reservestorage/headers_gc.go` (best-effort prune header height memberships below finalized)
### Modified
- `core/node/node.go` (prunes header memberships below finalized during TryAdvanceTip GC)



## v1.0.102 — 2026-03-02
### Modified
- `core/node/node.go` (/sync/status now reports header_tip and missing_blocks)



## v1.0.103 — 2026-03-02
### Added
- `internal/reservestorage/headers_index_test.go` (basic fork-aware header index test)



## v1.0.104 — 2026-03-02
### Added
- `core/node/node.go` (`GET /sync/cumwork?hash=` returns stored cumulative work)



## v1.0.105 — 2026-03-02
### Modified
- `README.md` (documented fork-aware headers + best-chain sync flow)



## v1.0.106 — 2026-03-02
### Added
- `core/node/bestchain.go` (best-chain hash lookup by height)
- `core/node/node.go` (`TryCommitNext`, `POST /sync/commit`)

### Modified
- `core/node/node.go` (replaced linear tip advance with best-chain-aware commit)
- `README.md`



## v1.0.107 — 2026-03-02
### Added
- `internal/reservestorage/bestchain_marker.go` (stores current best-chain tip hash)

### Modified
- `core/node/p2p_gossip.go` (updates best-chain marker on header-tip update)
- `core/node/bestchain.go` (uses best-chain marker when available)



## v1.0.108 — 2026-03-02
### Added
- `internal/reservestorage/peer_headers_state.go` (tracks per-peer last headers-from)

### Modified
- `core/node/p2p_gossip.go` (rejects header windows that move backwards for same peer)



## v1.0.109 — 2026-03-02
### Added
- `core/node/reorg.go` (state rebuild/replay scaffold; finalized-safe)
- `internal/reservestorage/state_import.go` (wipe/import state prefixes)
- `core/node/node.go` (`POST /sync/rebuild?height=`)

### Notes
- This is a first reorg/rebuild primitive: restore state to a height via snapshots + replay.



## v1.0.110 — 2026-03-02
### Added
- `config/node/node.json` (`finality.checkpoint_interval`)
- `core/node/config.go` (`FinalityConfig`)

### Modified
- `core/node/node.go` (rejects votes not on checkpoint heights)



## v1.0.111 — 2026-03-02
### Added
- `core/consensus/pow/retarget.go` (simple retarget placeholder)

### Notes
- Retarget is scaffolded; full bitcoin-style bits math can replace this later.



## v1.0.112 — 2026-03-02
### Notes
- Mempool nonce-lanes are enforced by the existing sim-state filter; this release adds documentation marker for future per-sender queues.



## v1.0.113 — 2026-03-02
### Added
- `core/node/reorg_txs.go` (scaffold for reorg/orphan tx reinsertion)



## v1.0.114 — 2026-03-02
### Added
- `core/node/integration_test.go` (integration harness placeholder)



## v1.0.115 — 2026-03-02
### Modified
- `core/node/node.go` (/metrics now includes header_tip_height and finalized_height)



## v1.0.116 — 2026-03-02
### Modified
- `core/node/config.go` (added config validation and fail-fast checks in LoadConfig)



## v1.0.117 — 2026-03-02
### Added
- `core/node/reorg_best.go` (finalized-safe reorg to best header chain using snapshots+replay)
- `core/node/node.go` (`POST /sync/reorg_best`)
### Notes
- Requires blocks for the target best header chain to be present locally; otherwise returns missing_block error.



## v1.0.118 — 2026-03-02
### Added
- `core/node/node.go` (`TryConvergeToBest`, `POST /sync/converge`)

### Notes
- `TryConvergeToBest` is a convenience orchestration for testing multi-peer sync convergence.



## v1.0.119 — 2026-03-02
### Added
- `config/node/node.json` (`sync.auto_converge`, `sync.auto_converge_interval_sec`)

### Modified
- `core/node/config.go` (SyncConfig auto converge fields)
- `core/node/sync_manager.go` (periodic auto converge loop)
- `README.md`



## v1.0.120 — 2026-03-02
### Modified
- `core/node/reorg_best.go` (reinsert non-coinbase txs from orphaned old branch back into mempool after reorg)



## v1.0.121 — 2026-03-02
### Modified
- `core/node/node.go` (slashing: vote blockhash must exist and match vote height)



## v1.0.122 — 2026-03-02
### Added
- `internal/reservestorage/peer_headers_end.go` (per-peer last headers end height)
### Modified
- `core/node/p2p_gossip.go` (rejects overlapping/non-advancing headers windows per peer)



## v1.0.123 — 2026-03-02
### Modified
- `core/node/node.go` (/metrics adds sync pending/inflight + p2p peers)



## v1.0.124 — 2026-03-02
### Added
- `core/node/node.go` (`POST /sync/auto_converge` runtime toggle)



## v1.0.125 — 2026-03-02
### Modified
- `core/node/sync_manager.go` (auto converge loop respects sync pause)



## v1.0.126 — 2026-03-02
### Added
- `core/node/node.go` (`GET /sync/state_height` alias for current canonical tip)



## v1.0.127 — 2026-03-02
### Added
- `core/node/bestchain_test.go` (basic best-chain marker test)



## v1.0.128 — 2026-03-02
### Modified
- `core/node/p2p_gossip.go` (rejects blocks where message hash != computed hash)



## v1.0.129 — 2026-03-02
### Modified
- `README.md` (sync hardening notes)



## v1.0.130 — 2026-03-02
### Added
- `config/node/node.json` (`sync.inflight_timeout_sec`)
### Modified
- `core/node/config.go` (SyncConfig.InFlightTimeoutSec)
- `core/node/sync_manager.go` (requeues timed-out in-flight block hashes)



## v1.0.131 — 2026-03-02
### Added
- `core/node/node.go` (`POST /sync/retry` forces inflight requeue + converge)



## v1.0.132 — 2026-03-02
### Added
- `internal/reservestorage/peer_getblock.go` (dedupe get_block per peer+hash)
### Modified
- `core/node/sync_manager.go` (avoids spamming get_block requests repeatedly to same peer)



## v1.0.133 — 2026-03-02
### Modified
- `core/node/node.go` (/sync/status now includes best header hash + best cumwork)



## v1.0.134 — 2026-03-02
### Added
- `core/node/node.go` (`GET /sync/reorg_best_check` dry-run presence check)



## v1.0.135 — 2026-03-02
### Added
- `config/node/node.json` (`mempool.max_txs_per_sender`)
### Modified
- `core/node/config.go` (MempoolConfig.MaxTxsPerSender)
- `core/node/mempool.go` (enforces per-sender cap in Add)



## v1.0.136 — 2026-03-02
### Added
- `config/node/node.json` (`pow.retarget_interval`, `pow.target_block_sec`)
### Modified
- `core/node/config.go` (PowConfig retarget fields)
- `core/node/miner.go` (applies simple retarget at interval boundaries)



## v1.0.137 — 2026-03-02
### Modified
- `core/node/node.go` (/finality/tally checkpoint_id now includes height:hash)



## v1.0.138 — 2026-03-02
### Modified
- `core/node/integration_test.go` (slightly expanded compile-only harness)



## v1.0.139 — 2026-03-02
### Modified
- `README.md` (documented inflight timeout + retry)



## v1.0.140 — 2026-03-02
### Added
- `core/chain/hash_parse.go` (parse hex -> chain.Hash)
- `internal/reservestorage/bestchain_cache.go` (best-chain height->hash cache)

### Modified
- `core/node/p2p_gossip.go` (validates headers meet PoW target; fills best-chain cache on best tip update)
- `core/node/bestchain.go` (uses cached best hash at height when available)
- `internal/reservestorage/header_ancestors.go` (PrevHash handling)



## v1.0.141 — 2026-03-02
### Added
- `core/node/block_header_match.go` (ensures blocks match stored headers)

### Modified
- `core/node/node.go` (`TryCommitNext` now requires stored header exists and block fields match before advancing tip)



## v1.0.142 — 2026-03-02
### Added
- `config/node/node.json` (`pow.max_future_drift_sec`)
### Modified
- `core/node/config.go` (PowConfig.MaxFutureDriftSec)
- `core/node/p2p_gossip.go` (reject headers too far in the future)



## v1.0.143 — 2026-03-02
### Modified
- `core/node/p2p_gossip.go` (header bits continuity rule between retarget boundaries)



## v1.0.144 — 2026-03-02
### Modified
- `core/node/reorg_best.go` (forkpoint detection uses best-chain cache by height when available)



## v1.0.145 — 2026-03-02
### Added
- `internal/reservestorage/finality_finalized_hash.go` (stores finalized checkpoint hash)

### Modified
- `core/node/finality_weighted.go` (records finalized hash when finalizing)
- `core/node/node.go` (/finality/finalized now includes finalized_hash)



## v1.0.146 — 2026-03-02
### Modified
- `core/node/p2p_gossip.go` (P2P hello/get_hello chain_id check; requests hello during gossip loop)



## v1.0.147 — 2026-03-02
### Added
- `core/node/sync_manager.go` (placeholder prioritizeNearTip hook)



## v1.0.148 — 2026-03-02
### Notes
- Added explicit eviction TODO marker.



## v1.0.149 — 2026-03-02
### Added
- `core/consensus/pow/verify_test.go`



## v1.0.150 — 2026-03-02
### Modified
- `core/node/node.go` (TryCommitNext now applies block state before advancing tip)



## v1.0.151 — 2026-03-02
### Added
- `core/node/node.go` (`POST /sync/commit_until?n=`)



## v1.0.152 — 2026-03-02
### Modified
- `core/node/reorg_best.go` (reorg apply requires block matches stored header)



## v1.0.153 — 2026-03-02
### Modified
- `core/node/node.go` (/finality/anchor now includes finalized_hash)



## v1.0.154 — 2026-03-02
### Added
- `config/node/node.json` (`chain.genesis_hash`)
### Modified
- `core/node/config.go` (ChainConfig.GenesisHash)
- `core/node/p2p_gossip.go` (hello includes/checks genesis_hash)



## v1.0.155 — 2026-03-02
### Added
- `core/node/header_validation_test.go` (skeleton test for header store)



## v1.0.156 — 2026-03-02
### Modified
- `README.md` (documented commit_until and retry)

## v1.0.157 — 2026-03-02
### Added (GitHub repo scaffolding)
- `.gitignore`
- `LICENSE`
- `CONTRIBUTING.md`
- `CODE_OF_CONDUCT.md`
- `SECURITY.md`
- `.editorconfig`
- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`
- `.github/dependabot.yml`
- `.github/pull_request_template.md`
- `.github/ISSUE_TEMPLATE/bug_report.yml`
- `.github/ISSUE_TEMPLATE/feature_request.yml`
- `.github/ISSUE_TEMPLATE/config.yml`

### Modified
- `README.md` (Quick start + GitHub notes)

