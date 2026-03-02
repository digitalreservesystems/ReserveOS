#!/usr/bin/env bash
set -euo pipefail
mkdir -p build/ReserveOS
echo "Building gateway binaries..."
go build -o build/ReserveOS/initialize-daemon ./services/initialize-daemon
go build -o build/ReserveOS/node ./core/cmd/node
go build -o build/ReserveOS/wallet-daemon ./services/wallet-daemon
go build -o build/ReserveOS/platformdb-daemon ./services/platformdb-daemon
echo "Done. Run: build/ReserveOS/initialize-daemon config/gateway/gateway.json"
