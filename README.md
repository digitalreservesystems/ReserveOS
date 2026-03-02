# ReserveOS OTAP + PoS Finality Demo ZIP

This build includes:
- PoW block production (mining) + difficulty retarget
- OTAP (One-Time Address Payments): outputs (P, R, tag, enc_memo)
- Wallet scans blocks, detects OTAP outputs, decrypts memo to recover slot_id, and marks confirmed
- PoS checkpoint finality scaffold: validators sign checkpoint votes every N blocks; finalized height advances when threshold is met

## Run order
1) Node
2) storage-daemon
3) wallet-daemon

## 0) Set Keymaster (required)
Windows PowerShell:
```powershell
$env:RESERVEOS_KEYMASTER="dev-secret"
```

## 1) Run node
```powershell
go run .\core\cmd\node config\node\node.json
```

Useful endpoints:
- GET  http://127.0.0.1:18445/chain/info
- POST http://127.0.0.1:18445/tx/submit
- POST http://127.0.0.1:18445/chain/mine_one
- GET  http://127.0.0.1:18445/finality/info
- GET  http://127.0.0.1:18445/finality/validators

Notes:
- The node auto-generates an Ed25519 validator keypair into keyvault on first run (pos.ed25519_pub/priv).
- Checkpoints occur every 20 blocks by default and are finalized once votes reach >= 2/3 total weight.

## 2) Run storage-daemon
```powershell
go run .\services\storage-daemon config\services\storage-daemon\service.json
```

## 3) Run wallet-daemon (OTAP receiver)
Wallet uses keyvault for OTAP keys. On first run, it generates OTAP keys and stores them in keyvault.

```powershell
go run .\services\wallet-daemon config\services\wallet-daemon\service.json
```

Endpoints:
- POST http://127.0.0.1:9011/otap/request  -> alloc slot_id + returns S_pub,V_pub
- POST http://127.0.0.1:9011/otap/build_tx -> builds a tx JSON with OTAP output

## End-to-end OTAP test

1) Request slot + receiver registry keys:
```powershell
curl -X POST http://127.0.0.1:9011/otap/request -H "Content-Type: application/json" -d '{"purpose":"receive","expires_in":600}'
```

2) Build OTAP payment tx (paste keys and slot_id from step 1):
```powershell
curl -X POST http://127.0.0.1:9011/otap/build_tx -H "Content-Type: application/json" -d '{"recipient":{"S_pub":"PASTE","V_pub":"PASTE"},"amount":1000,"asset":"USDR","slot_id":SLOT_ID,"note":"invoice-123"}'
```

3) Submit the returned JSON to node:
```powershell
curl -X POST http://127.0.0.1:18445/tx/submit -H "Content-Type: application/json" -d @tx.json
```

4) Mine blocks:
```powershell
curl -X POST http://127.0.0.1:18445/chain/mine_one
```

## PoS finality quick test
Mine up to height 20:
```powershell
for ($i=0; $i -lt 20; $i++) {{ curl -s -X POST http://127.0.0.1:18445/chain/mine_one | Out-Null }}
curl http://127.0.0.1:18445/finality/info
```

With the default single local validator weight=100, the checkpoint at height 20 will be finalized immediately.


## Multi-validator vote ingestion (new)
- POST http://127.0.0.1:18445/finality/submit_vote  (JSON VoteRecord)
- GET  http://127.0.0.1:18445/finality/votes?height=20

Genesis now includes `validators` + `finality` fields (used if config finality validators are empty).


## Finality vote gossip (automatic)
Configure peers in `config/node/node.json` under `finality.peers`.
Example:
  "peers": ["http://127.0.0.1:18446","http://127.0.0.1:18447"]
Nodes will periodically broadcast their checkpoint votes and pull votes from peers.


## Block sync over HTTP (new)
- Configure `sync.peers` to point to other nodes' base URLs.
- A node will pull `/chain/info` and then fetch missing heights via `/chain/block?height=N` and apply them.

Example config snippet:
  "sync": {"enabled": true, "peers": ["http://127.0.0.1:18446"], "interval_seconds": 3, "max_blocks_per_round": 50}


## Fork-choice + finalized reorg protection (new)
- Sync is now able to recover from mismatched tips by doing a **safe reorg-from-finalized**.
- Rule: **never reorg below locally finalized height**.
- If a peer has a different block hash at the local finalized height, reorg is rejected.


## Fork-choice (new)
- Node now accepts blocks that do NOT extend the current tip (fork blocks).
- Each block gets a 64-bit cumulative work score; if a fork has higher cumulative work and contains the finalized anchor, the node switches the main chain mapping.
- Height->hash mapping now represents the selected main chain; fork blocks remain retrievable by hash.


## New chain RPCs
- GET /chain/block_by_hash?hash=<hex>
- GET /chain/header_by_hash?hash=<hex>
- GET /chain/headers?from=<height>&count=<n>  (headers-first sync)


## True headers-first scoring (new)
- Sync now scores incoming headers (PoW + cumulative work) before fetching full blocks.
- Blocks are fetched only if the header chain improves cumulative work over current tip.
- `/chain/info` includes `tip_work`.


## Fork-point discovery + ancestor sync (new)
- Sync now supports peers whose best chain diverged behind your tip.
- It walks peer headers backwards by hash to find a local-known common ancestor, but never below finalized anchor.
- Then it downloads and applies blocks by hash along that branch.


## Block locator (new)
- Added `POST /chain/locate` which returns the first locator hash the peer recognizes (common ancestor).
- Sync uses a Bitcoin-style block locator (exponential backoff) to find common ancestor in O(log n) requests.


## USDR gas + fee policy (new)
- Transactions now include `fee` and `gas_asset`.
- Node enforces `fee >= estimate` (size-hint + OTAP multiplier + mode multiplier).
- Fee pools are tracked in LevelDB and exposed via GET /fees/info.
- On mining, fees are split into: validators / participation(PoP) / treasury / defense pool.


## PoP distribution (implemented)
- Register participants: POST /pop/register {"id":"alice","weight":1}
- List participants: GET /pop/participants
- PoP payouts happen every `pop.epoch_blocks` (default 50) and drain the participation fee pool into balances under `pop:<id>`.
- Check balances: GET /state/balance?id=pop:alice (or validators:pool, treasury:pool, defense:pool)


## Account txs (signed) (new)
- `/tx/submit` now requires `from`, `pubkey`, `sig`, and correct `nonce`.
- `from` is the ed25519 pubkey hex.
- Balances are tracked under `bal:<id>`; OTAP outputs credit `otap:<P>` buckets for later claim.
- NOTE: For forks, state is rebuilt after main-chain switches (demo approach).


## OTAP claim txs (new)
- OTAP outputs credit `otap:<P>` buckets.
- Claim by submitting a signed tx with `type:"otap_claim"` and `otap_claim:{p,r,to,amount}`.
- Fees are paid by the claimant from their normal balance; claimed funds move from `otap:<P>` into `to`.
- Check bucket: GET /otap/bucket?p=<P>


### OTAP claim proof (new)
- `otap_claim.claim_sig` is required.
- It must be an **Ed25519 signature made by the one-time private key** corresponding to `P`.
- The node verifies it against pubkey `P` over `tx.SigningBytes()` (both account sig and claim sig excluded from the message).


## Versioning
- Current version is in `VERSION`.
- Changes are recorded in `CHANGELOG.md`.


## Wallet auto-claim
- wallet-daemon will attempt to auto-submit an `otap_claim` tx after detecting an OTAP output.
- Claim signature uses Schnorr25519 over `tx.SigningBytes()` (canonical signing bytes).


## State endpoints (new)
- GET /state/nonce?id=<id>
- GET /state/info?id=<id>  (balance + nonce)


## Canonical transaction encoding (v1)
- Tx signing and txid now use `core/chain/CanonicalTxBytes()` (deterministic length-prefixed encoding).
- This removes JSON ordering/whitespace ambiguity.


## Mempool upgrades (v1.0.5)
- Per-account nonce-indexed mempool with fee-rate priority across eligible next-nonce txs.
- RBF: replacing same (from, nonce) requires >= configured fee bump.
- TTL eviction and max tx/bytes limits.


## Issuance (coinbase)
- Node now prepends a `type:"coinbase"` tx when mining blocks (configurable in `issuance`).
- Default rewards go to `miner:local` in asset USDR.


## Finality (v1.0.7)
- Node only auto-signs checkpoints if its local pubkey is present in the validator set.


## PoP scoring (v1.0.8)
- POST /pop/heartbeat {"id":"alice","delta":1} increments PoP score.
- Epoch payouts weight by score if present; scores reset each payout.
- GET /pop/score?id=alice


## Keyvault rotation (v1.0.9)
- Keyvault now stores meta `active_key_id` and `created_unix` and rotates encryption key after `keyvault.rotation_days`.


## platformdb-daemon (v1.0.10)
- New service: `services/platformdb-daemon` (SQLite; SQLCipher best-effort via DSN).
- Config: `config/services/platformdb-daemon/service.json`
- Endpoints: /kv/get?k=, POST /kv/set


## P2P handshake scaffold (v1.0.11)
- Node starts a TCP listener (default :18444) with a signed hello handshake.
- Identity keys stored in keyvault under `p2p.ed25519_*`.


## Mempool introspection
- GET /mempool/info (counts, bytes, per-account queued txs, mempool config)


## Gateway node (A: one port, path-based)
Run:
- `go run ./services/initialize-daemon config/gateway/gateway.json`
Then open:
- http://localhost:8080/ (marketing)
- http://localhost:8080/portal
- http://localhost:8080/wallet
- http://localhost:8080/api/healthz (proxied to node)

Note: set `RESERVEOS_KEYMASTER` in your environment before starting.


## Tx gossip over HTTP (v1.0.19)
- POST `/gossip/tx` to share a tx with peers; node forwards up to `gossip.max_hops`.
- Node broadcasts locally accepted txs to `gossip.peers`.
- GET `/gossip/info`.


## Gateway API routing (v1.0.20)
- Gateway now proxies:
  - `/api/node/*` -> core node
  - `/api/wallet/*` -> wallet-daemon
  - `/api/platformdb/*` -> platformdb-daemon
- Gateway provides `/api/healthz`.
- To enable platformdb writes through gateway, set `RESERVEOS_PLATFORMDB_TOKEN` (or config `security.platformdb_token_env`).


## Gateway network status (v1.0.21)
- GET `/api/network/status` aggregates node metrics (chain, mempool, gossip, p2p, fees) into one JSON.


## Block gossip over HTTP (v1.0.22)
- Nodes broadcast mined blocks to gossip peers via POST `/gossip/block`.
- Peers submit blocks to `SubmitBlock` and forward up to `gossip.max_hops`.


## Peer management (v1.0.23)
- POST `/peers/add` {"addr":"http://host:18445"}
- GET `/peers/list`
- Gossip now uses config peers + stored peers.


## Gateway TCP proxy (v1.0.24)
- Gateway can proxy P2P TCP port (default 18444) while still serving HTTP on 8080.
- Config: `config/gateway/gateway.json` -> `tcp`.


## Flat webfs hashed assets (v1.0.25)
- Any non-HTML files in a bundle dir are copied into runtime/web as `name.<hash>.ext`.
- HTML can reference them with `{{asset:filename.ext}}` placeholders.


## Build scripts + presets (v1.0.26)
- Build gateway binaries:
  - Linux: `bash scripts/build_gateway.sh`
  - Windows: `scripts\\build_gateway.bat`
- Presets: `config/gateway/presets.json` (gateway preset).


## Gateway rate limiting (v1.0.27)
- Simple in-memory token-bucket rate limit for `/api/*` (excludes /api/healthz and /api/network/status).


## Gateway peer control (v1.0.28)
- GET `/api/network/peers` (gateway wrapper for node `/peers/list`)
- POST `/api/network/peers/add` {"addr":"http://host:18445"} (gateway wrapper for node `/peers/add`)


## Node hardening (v1.0.29)
- PoS finality threshold now uses on-disk validator registry weights when present (ignores slashed validators).
- Blocks now enforce coinbase rules, tx signature validity, and (when extending main tip) deterministic in-memory state simulation for balances/nonces.


## Block assembly correctness (v1.0.30)
- Mining now filters candidate mempool txs using an in-memory balance/nonce simulation before inclusion.
- Debug endpoints:
  - GET /mempool/eligible
  - GET /mempool/queue


## Encrypted P2P tx/block gossip (v1.0.31)
- Node now broadcasts txs and blocks over encrypted TCP sessions (after signed hello).
- Sessions dial peers derived from `gossip.peers` hostnames on port 18444.
- New endpoint: GET /p2p/sessions


## P2P inv/get gossip (v1.0.32)
- Encrypted P2P now uses lightweight inventory messages:
  - inv_tx -> get_tx -> tx
  - inv_block -> get_block -> block
- Includes a basic per-peer message rate limiter.


## Tx index (v1.0.33)
- Node stores submitted and mined txs in a LevelDB tx index.
- GET /tx/get?txid=<id>
- P2P get_tx now serves from tx index.


## Peer scoring + bans (v1.0.34)
- Peer score increments on valid tx/block messages and decrements on invalid.
- Auto-ban 10 minutes if score <= -50.
- GET /p2p/peerstat?pub=<peerPub>
- POST /p2p/ban {"pub":"...","seconds":600}


## State snapshots (v1.0.35)
- Periodically writes gzipped JSON snapshots of balances+nonces to `runtime/snapshots`.
- Config: `state.snapshot_interval`, `state.snapshot_dir`.
- Endpoints:
  - GET /state/snapshots
  - POST /state/snapshot/trigger
- Reorg rebuild uses latest snapshot then replays remaining blocks.


## Node roles / observer mode (v1.0.36)
- Config: `node.role` and `node.read_only`.
- Observer mode disables:
  - tx submission (`/tx/submit`)
  - mining (`/mine/one`)
  - checkpoint auto-signing
- Endpoint: GET /node/role


## Weighted finality (v1.0.48)
- Finality now records `finality:finalized_height` when a checkpoint reaches >=2/3 validator weight.
- Votes below finalized height are rejected.
- Endpoints:
  - GET /finality/finalized


## Sync manager (v1.0.79)
- Headers-first loop queues block hashes and fetches blocks via P2P get_block.
- GET /sync/status shows last headers, pending, inflight counts.


## Sync anchor + workers (v1.0.80)
- Sync now requests P2P `get_anchor` and requires peers to match local finalized anchor before accepting headers/blocks (when finalized_height > 0).
- Config: `sync.workers`, `sync.max_inflight_blocks`


## Sync peer selection (v1.0.81)
- Sync block fetch now selects a peer session (prefers anchor-matched when finalized) and sends get_block to that peer only.
- Adds /sync/peers for debugging.


## Sync continuity guard (v1.0.82)
- P2P headers accepted only if resp.From == tip+1 and first header PrevHash matches current tip hash.
- Debug: POST /sync/reset


## Strict-linear sync (v1.0.84)
- New config: `sync.strict_linear` (default true). When enabled, the node only advances the tip if an incoming block extends the current tip (height == tip+1 and prevhash == tip hash). This prevents out-of-order tip jumps during P2P sync.


## Linear tip advancement from staged candidates (v1.0.85)
- Incoming blocks are staged as candidates by height.
- With `sync.strict_linear=true`, the node advances the canonical tip only when it can connect height-by-height (prevhash matches tip hash).
- Debug: POST /sync/advance


## Header index + deterministic missing-block scheduler (v1.0.90)
- Incoming P2P headers are stored in a header index (height->hash, hash->header).
- Node tracks a persistent header tip separate from block tip.
- Sync scheduler enqueues missing blocks deterministically from block tip+1..header tip using the header index.
- Endpoint: GET /sync/header_tip


## Best header chain selection (v1.0.94)
- Node computes approximate cumulative work from headers (using Bits->Work64) and selects the best header tip by cumulative work (ties by height).
- Finalized guard is applied when updating best header tip.
- Endpoint: GET /sync/best_header_tip


## Best-chain-based sync scheduling (v1.0.95)
- Missing-block scheduling now follows the best header chain by walking prev pointers from `header_tip` down to `block_tip+1`.
- New endpoint: GET /sync/plan?n=50


## Fork-aware headers + best-chain sync (v1.0.105)
- Header storage supports multiple forks per height (hdrh:<height>:<hash>). Best header tip is selected by cumulative work with finalized guard.
- Sync scheduling follows the best header chain and deterministically requests missing blocks.


## Commit pipeline (v1.0.106)
- Canonical tip advancement now requires the next block to match the best header chain hash at that height (when header is known).
- New endpoint: POST /sync/commit


## Reorg to best header chain (v1.0.117)
- Added `POST /sync/reorg_best` to attempt a finalized-safe reorg to the current best header chain tip (requires blocks present).


## Converge helper (v1.0.118)
- `POST /sync/converge` runs: enqueue missing blocks -> commit forward -> attempt finalized-safe reorg to best header chain if still diverged.


## Auto converge (v1.0.119)
- New config: `sync.auto_converge` (default true) and `sync.auto_converge_interval_sec` (default 3).
- Sync manager periodically calls `TryConvergeToBest()` to keep block tip converging toward best header chain.


## Sync hardening (v1.0.129)
- Added per-peer headers window overlap protection, block hash mismatch checks, runtime auto_converge toggle, and richer metrics.


## New sync knobs (v1.0.139)
- `sync.inflight_timeout_sec` requeues stuck in-flight blocks.
- `sync.max_queue_fill` limits per-tick enqueue.
- `POST /sync/retry` forces a retry/converge cycle.


## New sync endpoints (v1.0.156)
- POST /sync/commit_until?n=100 commits multiple blocks.
- POST /sync/retry requeues inflight timeouts and runs converge.

## Quick start
```bash
go mod tidy
go test ./...
go run ./cmd/reserveos-node --config config/node/node.json
```

## GitHub
This repo includes:
- GitHub Actions CI (`.github/workflows/ci.yml`)
- Release build workflow (`.github/workflows/release.yml`)
- Issue + PR templates
- Dependabot for Go modules

