@echo off
setlocal
set PROFILE=%1
if "%PROFILE%"=="" set PROFILE=gateway
call scripts\apply_preset.bat %PROFILE%
if "%PROFILE%"=="gateway" (
  call scripts\build_gateway.bat
) else (
  if not exist build\ReserveOS mkdir build\ReserveOS
  go build -o build\ReserveOS\node.exe .\core\cmd\node
)
echo Built profile: %PROFILE%
endlocal
