<#
.SYNOPSIS
  NginxPanel-Lite 本地开发/测试一键脚本（无需 Docker）。

.DESCRIPTION
  - dev      （默认）后端 dry-run + 前端热更新，各开一个窗口，自动打开浏览器
  - backend  仅在当前窗口运行后端（go run，dry-run，:8080）
  - frontend 仅在当前窗口运行前端（vite，:5173，/api 代理到 :8080）
  - build    构建前端并内嵌进 Go 单体二进制后运行（:8080，模拟生产形态）
  - stop     结束占用 8080 / 5173 端口的进程

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File .\dev.ps1
  powershell -ExecutionPolicy Bypass -File .\dev.ps1 backend
  powershell -ExecutionPolicy Bypass -File .\dev.ps1 build
  powershell -ExecutionPolicy Bypass -File .\dev.ps1 stop
#>

[CmdletBinding()]
param(
  [Parameter(Position = 0)]
  [ValidateSet('dev', 'backend', 'frontend', 'build', 'stop')]
  [string]$Mode = 'dev'
)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$web = Join-Path $root 'web'
$dataDir = Join-Path $root 'data\dev'   # 位于被 .gitignore 忽略的 data/ 下

# 开发环境变量：dry-run 跳过 docker，关闭 Secure Cookie 以便走 http，固定密钥避免重启掉登录态
$devEnv = [ordered]@{
  PANEL_NGINX_DRYRUN  = 'true'
  PANEL_COOKIE_SECURE = 'false'
  PANEL_DATA_DIR      = $dataDir
  PANEL_LISTEN_ADDR   = ':8080'
  PANEL_JWT_SECRET    = 'local-dev-secret-change-me-0123456789'
  ACME_STAGING        = 'true'
}

function Assert-Tool($name, $hint) {
  if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
    throw "未找到 $name，请先安装或加入 PATH。$hint"
  }
}

function Set-DevEnv {
  foreach ($k in $devEnv.Keys) { Set-Item -Path "env:$k" -Value $devEnv[$k] }
}

function Ensure-WebDeps {
  if (-not (Test-Path (Join-Path $web 'node_modules'))) {
    Write-Host '[frontend] 首次运行，安装依赖 (npm install)...' -ForegroundColor Cyan
    Push-Location $web
    try { npm install } finally { Pop-Location }
  }
}

function Wait-Backend {
  Write-Host '[backend] 等待后端就绪（首次需编译，可能十几秒）...' -ForegroundColor Cyan
  for ($i = 0; $i -lt 90; $i++) {
    try {
      $r = Invoke-WebRequest -Uri 'http://localhost:8080/api/system/health' -UseBasicParsing -TimeoutSec 2
      if ($r.StatusCode -eq 200) { Write-Host '[backend] 就绪 ✓' -ForegroundColor Green; return $true }
    } catch { Start-Sleep -Seconds 1 }
  }
  Write-Host '[backend] 等待超时，请查看后端窗口的日志' -ForegroundColor Yellow
  return $false
}

function Start-BackendWindow {
  $envLines = ($devEnv.GetEnumerator() | ForEach-Object { "`$env:$($_.Key)='$($_.Value)'" }) -join '; '
  $inner = "$envLines; Write-Host '后端运行中 http://localhost:8080  (dry-run：nginx 操作被模拟，不需要 docker)' -ForegroundColor Green; go run ./cmd/server"
  Start-Process powershell -ArgumentList '-NoExit', '-Command', $inner -WorkingDirectory $root | Out-Null
}

function Start-FrontendWindow {
  $inner = "Write-Host '前端运行中 http://localhost:5173  (热更新)' -ForegroundColor Green; npm run dev"
  Start-Process powershell -ArgumentList '-NoExit', '-Command', $inner -WorkingDirectory $web | Out-Null
}

function Stop-DevPorts {
  foreach ($port in 8080, 5173) {
    try {
      $conns = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
      foreach ($c in ($conns | Select-Object -ExpandProperty OwningProcess -Unique)) {
        Stop-Process -Id $c -Force -ErrorAction SilentlyContinue
        Write-Host "已结束占用端口 $port 的进程 (PID $c)" -ForegroundColor Yellow
      }
    } catch {}
  }
  Write-Host '完成。' -ForegroundColor Green
}

switch ($Mode) {

  'dev' {
    Assert-Tool go   '安装 Go 1.26+。'
    Assert-Tool node '安装 Node.js 20+。'
    Assert-Tool npm  '随 Node.js 一起安装。'
    New-Item -ItemType Directory -Force $dataDir | Out-Null
    Ensure-WebDeps

    Write-Host '启动后端窗口...' -ForegroundColor Cyan
    Start-BackendWindow
    Wait-Backend | Out-Null

    Write-Host '启动前端窗口...' -ForegroundColor Cyan
    Start-FrontendWindow
    Start-Sleep -Seconds 3
    Start-Process 'http://localhost:5173'

    Write-Host ''
    Write-Host '==================================================' -ForegroundColor Green
    Write-Host '  前端(访问这个):  http://localhost:5173' -ForegroundColor Green
    Write-Host '  后端 API:        http://localhost:8080/api' -ForegroundColor DarkGray
    Write-Host '--------------------------------------------------' -ForegroundColor Green
    Write-Host '  首次访问 -> 初始化管理员 -> 用 Authenticator 扫码绑定 TOTP'
    Write-Host '  dry-run：可创建/编辑站点并「查看生成配置」，但不会真正下发到 nginx'
    Write-Host '  停止：关闭两个弹出窗口，或运行  .\dev.ps1 stop'
    Write-Host '==================================================' -ForegroundColor Green
  }

  'backend' {
    Assert-Tool go '安装 Go 1.26+。'
    New-Item -ItemType Directory -Force $dataDir | Out-Null
    Set-DevEnv
    Write-Host '后端运行中 http://localhost:8080  (dry-run，Ctrl+C 停止)' -ForegroundColor Green
    Push-Location $root
    try { go run ./cmd/server } finally { Pop-Location }
  }

  'frontend' {
    Assert-Tool npm '随 Node.js 一起安装。'
    Ensure-WebDeps
    Write-Host '前端运行中 http://localhost:5173  (需要后端在 :8080，Ctrl+C 停止)' -ForegroundColor Green
    Push-Location $web
    try { npm run dev } finally { Pop-Location }
  }

  'build' {
    Assert-Tool go  '安装 Go 1.26+。'
    Assert-Tool npm '随 Node.js 一起安装。'
    Ensure-WebDeps

    Write-Host '[build] 构建前端...' -ForegroundColor Cyan
    Push-Location $web
    try { npm run build } finally { Pop-Location }

    $embed = Join-Path $root 'internal\web\dist'
    Write-Host '[build] 复制前端产物到 embed 目录...' -ForegroundColor Cyan
    Remove-Item (Join-Path $embed 'assets') -Recurse -Force -ErrorAction SilentlyContinue
    Copy-Item (Join-Path $web 'dist\*') $embed -Recurse -Force

    Write-Host '[build] 编译单体二进制 panel.exe...' -ForegroundColor Cyan
    Push-Location $root
    try { go build -tags 'osusergo,netgo' -o panel.exe ./cmd/server } finally { Pop-Location }

    New-Item -ItemType Directory -Force $dataDir | Out-Null
    Set-DevEnv
    Write-Host '单体运行 http://localhost:8080  (前端已内嵌，Ctrl+C 停止)' -ForegroundColor Green
    Write-Host '提示：embed 目录已被替换为真实构建，恢复占位文件可执行  git checkout internal/web/dist/index.html' -ForegroundColor DarkGray
    Start-Sleep -Seconds 1
    Start-Process 'http://localhost:8080'
    & (Join-Path $root 'panel.exe')
  }

  'stop' {
    Stop-DevPorts
  }
}
