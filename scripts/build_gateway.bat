@echo off
setlocal
if not exist build\ReserveOS mkdir build\ReserveOS
echo Building gateway binaries...
go build -o build\ReserveOS\initialize-daemon.exe .\services\initialize-daemon
go build -o build\ReserveOS\node.exe .\core\cmd\node
go build -o build\ReserveOS\wallet-daemon.exe .\services\wallet-daemon
go build -o build\ReserveOS\platformdb-daemon.exe .\services\platformdb-daemon
echo Done. Run: build\ReserveOS\initialize-daemon.exe config\gateway\gateway.json
endlocal
