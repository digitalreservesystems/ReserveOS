@echo off
setlocal
set PRESET=%1
if "%PRESET%"=="" set PRESET=gateway
set NODECFG=config\node\node.json
set PRESETS=config\node\presets.json
python -c "import json,sys; p='%PRESET%'; cfg=json.load(open(r'%NODECFG%')); presets=json.load(open(r'%PRESETS%')); patch=presets.get(p); assert patch, 'unknown preset'; cfg.setdefault('node',{}); cfg['node'].update(patch.get('node',{})); json.dump(cfg, open(r'%NODECFG%','w'), indent=2); print('applied',p)"
endlocal
