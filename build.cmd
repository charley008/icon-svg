@echo off
setlocal
cd /d "%~dp0"
if not exist "bin" mkdir "bin"
go build -trimpath -ldflags "-s -w" -o "bin\icon-svg.exe" .
exit /b %errorlevel%
