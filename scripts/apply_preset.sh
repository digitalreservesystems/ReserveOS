#!/usr/bin/env bash
set -euo pipefail
PRESET="${1:-gateway}"
NODECFG="config/node/node.json"
PRESETS="config/node/presets.json"
python - <<'PY'
import json,sys
preset=sys.argv[1]
nodecfg=sys.argv[2]
presets=json.load(open(sys.argv[3],'r',encoding='utf-8'))
cfg=json.load(open(nodecfg,'r',encoding='utf-8'))
patch=presets.get(preset)
if not patch:
    raise SystemExit(f"unknown preset: {preset}")
# shallow merge for node section
cfg.setdefault("node",{})
for k,v in patch.get("node",{}).items():
    cfg["node"][k]=v
json.dump(cfg, open(nodecfg,'w',encoding='utf-8'), indent=2)
print("applied", preset, "to", nodecfg)
PY "$PRESET" "$NODECFG" "$PRESETS"
