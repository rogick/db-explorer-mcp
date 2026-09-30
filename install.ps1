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

function Prompt-YesNo([string]$message, [bool]$defaultYes = $true) {
    $suffix = if ($defaultYes) { "(s/n) [Padrão: s]" } else { "(s/n) [Padrão: n]" }
    $resp = Read-Host "$message $suffix"
    if ([string]::IsNullOrWhiteSpace($resp)) {
        return $defaultYes
    }
    return ($resp -match '^(s|sim|y|yes)$')
}

function Convert-PsObjectToHashtable($obj) {
    if ($null -eq $obj) { return $null }
    if ($obj -is [System.Collections.IDictionary]) { return $obj }
    if ($obj -is [System.Collections.IEnumerable] -and $obj -isnot [string]) {
        $list = [System.Collections.ArrayList]::new()
        foreach ($item in $obj) {
            $list.Add((Convert-PsObjectToHashtable $item)) | Out-Null
        }
        return $list
    }
    if ($obj -is [PSCustomObject]) {
        $hash = [ordered]@{}
        foreach ($prop in $obj.PSObject.Properties) {
            $hash[$prop.Name] = Convert-PsObjectToHashtable $prop.Value
        }
        return $hash
    }
    return $obj
}

function Update-McpJsonConfig([string]$filePath, [string]$serverPath) {
    $dir = Split-Path -Parent $filePath
    if (-not [string]::IsNullOrEmpty($dir) -and -not (Test-Path $dir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }

    $hash = [ordered]@{}
    if (Test-Path $filePath) {
        try {
            $raw = Get-Content -Path $filePath -Raw -Encoding UTF8 -ErrorAction Stop
            if (-not [string]::IsNullOrWhiteSpace($raw)) {
                $obj = $raw | ConvertFrom-Json
                if ($obj) {
                    $hash = Convert-PsObjectToHashtable $obj
                }
            }
        } catch {
            $hash = [ordered]@{}
        }
    }
    if (-not $hash) { $hash = [ordered]@{} }

    if (-not $hash.Contains("mcpServers") -or -not ($hash["mcpServers"] -is [System.Collections.IDictionary])) {
        $hash["mcpServers"] = [ordered]@{}
    }

    $hash["mcpServers"]["db-explorer"] = [ordered]@{
        "command" = $serverPath
    }

    $json = $hash | ConvertTo-Json -Depth 10
    [System.IO.File]::WriteAllText($filePath, $json, [System.Text.Encoding]::UTF8)
}

function Update-CodexTomlConfig([string]$filePath, [string]$serverPath) {
    $dir = Split-Path -Parent $filePath
    if (-not [string]::IsNullOrEmpty($dir) -and -not (Test-Path $dir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }

    $escapedPath = $serverPath -replace '\\', '\\'
    $block = @"
[mcp_servers.db-explorer]
command = "$escapedPath"
enabled = true
"@

    if (Test-Path $filePath) {
        $content = Get-Content -Path $filePath -Raw -Encoding UTF8
        if ($null -eq $content) { $content = "" }

        $pattern = '(?m)^\[mcp_servers\.db-explorer\](\r?\n(?:[^\[\r\n].*)?)*'
        if ($content -match $pattern) {
            $newContent = [regex]::Replace($content, $pattern, ($block.Trim() + [Environment]::NewLine))
            [System.IO.File]::WriteAllText($filePath, $newContent, [System.Text.Encoding]::UTF8)
        } else {
            $prefix = if ([string]::IsNullOrWhiteSpace($content)) { "" } else { [Environment]::NewLine + [Environment]::NewLine }
            [System.IO.File]::AppendAllText($filePath, $prefix + $block.Trim() + [Environment]::NewLine, [System.Text.Encoding]::UTF8)
        }
    } else {
        [System.IO.File]::WriteAllText($filePath, $block.Trim() + [Environment]::NewLine, [System.Text.Encoding]::UTF8)
    }
}

Write-Host ""
Write-Host "==================================================" -ForegroundColor Cyan
Write-Host " Configuração dos Clientes de IA / MCP" -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

# 1. Claude Code
Write-Host ""
Write-Host "--- 1. Claude Code (CLI) ---" -ForegroundColor Cyan
if (Prompt-YesNo "Deseja registrar o MCP no Claude Code?") {
    Write-Host "Escolha o escopo de ativação no Claude Code:" -ForegroundColor White
    Write-Host "  [1] Global / User (Disponível em qualquer projeto) [Padrão]" -ForegroundColor Green
    Write-Host "  [2] Local (Apenas para a pasta atual)" -ForegroundColor White
    $scopeChoice = Read-Host "Escolha o escopo [Padrão: 1]"
    $scope = if ($scopeChoice -eq "2") { "local" } else { "user" }

    if (Get-Command "claude" -ErrorAction SilentlyContinue) {
        Write-Host ">> Registrando MCP 'db-explorer' no Claude Code com escopo '$scope'..." -ForegroundColor Green
        try {
            & claude mcp remove --scope $scope db-explorer 2>$null | Out-Null
        } catch { }

        & claude mcp add --scope $scope db-explorer -- "$serverExePath"
        if ($LASTEXITCODE -eq 0) {
            Write-Host "✅ Registrado no Claude Code com sucesso!" -ForegroundColor Green
        } else {
            Write-Host "⚠️ Ocorreu um aviso durante a execução do claude mcp add." -ForegroundColor Yellow
            Write-Host "Comando manual: claude mcp add --scope $scope db-explorer -- `"$serverExePath`"" -ForegroundColor Cyan
        }
    } else {
        Write-Host "⚠️ Comando 'claude' não encontrado no PATH atual." -ForegroundColor Yellow
        Write-Host "Após instalar o Claude Code CLI, você pode registrar executando:" -ForegroundColor White
        Write-Host "  claude mcp add --scope $scope db-explorer -- `"$serverExePath`"" -ForegroundColor Cyan
    }
} else {
    Write-Host "⏭️ Registro no Claude Code ignorado." -ForegroundColor DarkGray
}

# 2. Claude Desktop
Write-Host ""
Write-Host "--- 2. Claude Desktop ---" -ForegroundColor Cyan
if (Prompt-YesNo "Deseja configurar o MCP no Claude Desktop?") {
    $claudeDesktopPath = Join-Path $env:APPDATA "Claude\claude_desktop_config.json"
    try {
        Update-McpJsonConfig $claudeDesktopPath $serverExePath
        Write-Host "✅ Configuração adicionada ao Claude Desktop com sucesso!" -ForegroundColor Green
        Write-Host "   Arquivo: $claudeDesktopPath" -ForegroundColor DarkGray
    } catch {
        Write-Host "⚠️ Erro ao atualizar arquivo de configuração do Claude Desktop: $_" -ForegroundColor Yellow
        Write-Host "Configuração manual em '$claudeDesktopPath':" -ForegroundColor White
        Write-Host "  `"db-explorer`": { `"command`": `"$($serverExePath -replace '\\', '\\')`" }" -ForegroundColor Gray
    }
} else {
    Write-Host "⏭️ Configuração no Claude Desktop ignorada." -ForegroundColor DarkGray
}

# 3. Cursor
Write-Host ""
Write-Host "--- 3. Cursor ---" -ForegroundColor Cyan
if (Prompt-YesNo "Deseja configurar o MCP no Cursor?") {
    $cursorConfigPath = Join-Path $env:USERPROFILE ".cursor\mcp.json"
    try {
        Update-McpJsonConfig $cursorConfigPath $serverExePath
        Write-Host "✅ Configuração adicionada ao Cursor com sucesso!" -ForegroundColor Green
        Write-Host "   Arquivo: $cursorConfigPath" -ForegroundColor DarkGray
    } catch {
        Write-Host "⚠️ Erro ao atualizar arquivo de configuração do Cursor: $_" -ForegroundColor Yellow
        Write-Host "Configuração manual em '$cursorConfigPath':" -ForegroundColor White
        Write-Host "  `"db-explorer`": { `"command`": `"$($serverExePath -replace '\\', '\\')`" }" -ForegroundColor Gray
    }
} else {
    Write-Host "⏭️ Configuração no Cursor ignorada." -ForegroundColor DarkGray
}

# 4. Codex
Write-Host ""
Write-Host "--- 4. Codex (CLI / Desktop) ---" -ForegroundColor Cyan
if (Prompt-YesNo "Deseja configurar o MCP no Codex?") {
    $codexTomlPath = Join-Path $env:USERPROFILE ".codex\config.toml"
    $codexRegistered = $false

    if (Get-Command "codex" -ErrorAction SilentlyContinue) {
        Write-Host ">> Registrando MCP 'db-explorer' via Codex CLI..." -ForegroundColor Green
        try {
            & codex mcp remove db-explorer 2>$null | Out-Null
        } catch { }

        & codex mcp add db-explorer -- "$serverExePath"
        if ($LASTEXITCODE -eq 0) {
            Write-Host "✅ Registrado no Codex CLI com sucesso!" -ForegroundColor Green
            $codexRegistered = $true
        }
    }

    try {
        Update-CodexTomlConfig $codexTomlPath $serverExePath
        if (-not $codexRegistered) {
            Write-Host "✅ Configuração gravada em: $codexTomlPath" -ForegroundColor Green
            if (-not (Get-Command "codex" -ErrorAction SilentlyContinue)) {
                Write-Host "ℹ️ Comando 'codex' não detectado no PATH. O arquivo $codexTomlPath foi atualizado diretamente." -ForegroundColor Yellow
            }
        }
    } catch {
        Write-Host "⚠️ Erro ao atualizar arquivo de configuração do Codex ($codexTomlPath): $_" -ForegroundColor Yellow
    }
} else {
    Write-Host "⏭️ Configuração no Codex ignorada." -ForegroundColor DarkGray
}

Write-Host ""
Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "✨ Instalação e configurações concluídas!" -ForegroundColor Cyan
Write-Host "Você pode gerenciar as conexões de banco executando:" -ForegroundColor White
Write-Host "  db-explorer-manager.exe (ou `"$managerExePath`")" -ForegroundColor Green
Write-Host "==================================================" -ForegroundColor Cyan
