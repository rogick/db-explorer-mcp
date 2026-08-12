$ErrorActionPreference = "Stop"

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host " DB Explorer MCP (Go) - Instalador para Windows" -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

# 1. Verificar compilador Go
if (-not (Get-Command "go" -ErrorAction SilentlyContinue)) {
    Write-Host "ERRO: O compilador Go não foi encontrado no PATH. Por favor, instale o Go 1.22+." -ForegroundColor Red
    exit 1
}

# 2. Definir diretório de instalação (~/.local/bin por padrão)
$targetBinDir = if ($env:BIN_DIR) { $env:BIN_DIR } else { Join-Path $env:USERPROFILE ".local\bin" }
New-Item -ItemType Directory -Force -Path $targetBinDir | Out-Null

Write-Host "[!] Compilando binários Go sem dependências externas (CGO_ENABLED=0)..." -ForegroundColor Green
$env:CGO_ENABLED = "0"

$serverExePath = Join-Path $targetBinDir "db-explorer-mcp.exe"
$managerExePath = Join-Path $targetBinDir "db-explorer-manager.exe"

# Também gerar na pasta local build para conveniência
$localBuildDir = Join-Path $PSScriptRoot "build"
New-Item -ItemType Directory -Force -Path $localBuildDir | Out-Null

go build -o "$serverExePath" "$PSScriptRoot\cmd\db-explorer-mcp\main.go"
go build -o "$managerExePath" "$PSScriptRoot\cmd\db-explorer-manager\main.go"

# Copiar para a pasta local build também
Copy-Item -Path "$serverExePath" -Destination "$localBuildDir\db-explorer-mcp.exe" -Force
Copy-Item -Path "$managerExePath" -Destination "$localBuildDir\db-explorer-manager.exe" -Force

Write-Host "✅ Binários instalados com sucesso em:" -ForegroundColor Green
Write-Host "  - $serverExePath" -ForegroundColor White
Write-Host "  - $managerExePath" -ForegroundColor White

# Verificar se a pasta de destino está no PATH do usuário
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not ($userPath -split ';' -contains $targetBinDir)) {
    Write-Host "⚠️ Nota: A pasta '$targetBinDir' não está no seu PATH de usuário." -ForegroundColor Yellow
}

Write-Host ""
Write-Host "=== Configuração no Claude CLI ===" -ForegroundColor Cyan

$defaultConfigDir = if ($env:CLAUDE_CONFIG_DIR) { $env:CLAUDE_CONFIG_DIR } else { Join-Path $env:USERPROFILE ".claude" }
$inputConfigDir = Read-Host "Digite o diretório de configuração do Claude [Padrão: $defaultConfigDir]"
if ([string]::IsNullOrWhiteSpace($inputConfigDir)) {
    $configDir = $defaultConfigDir
} else {
    $configDir = $inputConfigDir
}

$env:CLAUDE_CONFIG_DIR = $configDir

if (Get-Command "claude" -ErrorAction SilentlyContinue) {
    Write-Host ">> Registrando MCP no Claude CLI..." -ForegroundColor Green
    claude mcp add db-explorer "$serverExePath"
    Write-Host "✅ MCP 'db-explorer' instalado e registrado com sucesso!" -ForegroundColor Green
} else {
    Write-Host "⚠️ Comando 'claude' não encontrado no PATH." -ForegroundColor Yellow
    Write-Host "Comando gerado para execução manual:" -ForegroundColor Yellow
    Write-Host "  CLAUDE_CONFIG_DIR=`"$configDir`" claude mcp add db-explorer `"$serverExePath`"" -ForegroundColor White
}

Write-Host ""
Write-Host "=== Configuração no Claude Desktop (Windows) ===" -ForegroundColor Cyan
$claudeDesktopConfigPath = Join-Path $env:APPDATA "Claude\claude_desktop_config.json"
$exePathEscaped = $serverExePath -replace '\\', '\\'

Write-Host "Para usar no Claude Desktop no Windows, adicione a configuração abaixo ao seu arquivo:" -ForegroundColor White
Write-Host "  $claudeDesktopConfigPath" -ForegroundColor Yellow
Write-Host ""
Write-Host "{" -ForegroundColor Gray
Write-Host '  "mcpServers": {' -ForegroundColor Gray
Write-Host '    "db-explorer": {' -ForegroundColor Gray
Write-Host "      `"command`": `"$exePathEscaped`"" -ForegroundColor Gray
Write-Host "    }" -ForegroundColor Gray
Write-Host "  }" -ForegroundColor Gray
Write-Host "}" -ForegroundColor Gray

Write-Host ""
Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "✨ Instalação concluída com sucesso!" -ForegroundColor Cyan
Write-Host "Você pode gerenciar as conexões de banco executando:" -ForegroundColor White
Write-Host "  db-explorer-manager.exe (ou `"$managerExePath`")" -ForegroundColor Green
Write-Host "==================================================" -ForegroundColor Cyan
