@echo off
setlocal
cd /d %~dp0
set GOCACHE=%~dp0..\.gocache
go run .\runner\main.go %*
