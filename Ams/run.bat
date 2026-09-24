@echo off
chcp 65001 >nul
REM 一键启动：编译并运行资产管理系统。首次运行会自动建库并灌入演示数据。
setlocal

cd /d "%~dp0"

if "%AMS_ADDR%"=="" set AMS_ADDR=:8080

echo [1/2] 正在编译...
go build -o ams.exe .
if errorlevel 1 (
  echo 编译失败，请确认已安装 Go 1.22+
  exit /b 1
)

echo [2/2] 启动服务，浏览器访问 http://localhost%AMS_ADDR%
echo 按 Ctrl+C 退出
ams.exe
