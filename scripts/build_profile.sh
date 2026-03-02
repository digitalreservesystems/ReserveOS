#!/usr/bin/env bash
set -euo pipefail
PROFILE="${1:-gateway}"
bash scripts/apply_preset.sh "$PROFILE" || true
if [ "$PROFILE" = "gateway" ]; then
  bash scripts/build_gateway.sh
else
  mkdir -p build/ReserveOS
  go build -o build/ReserveOS/node ./core/cmd/node
fi
echo "Built profile: $PROFILE"
