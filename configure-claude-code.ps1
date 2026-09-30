$ErrorActionPreference = "Stop"

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host " Configuração do DB Explorer MCP no Claude Code (Windows) " -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan

# 1. Localizar os binários no diretório do script
$scriptDir = $PSScriptRoot
if (-not $scriptDir) {
    $scriptDir = (Get-Location).Path
}

$localServerExe = Join-Path $scriptDir "db-explorer-mcp.exe"
$localManagerExe = Join-Path $scriptDir "db-explorer-manager.exe"

if (-not (Test-Path $localServerExe)) {
    Write-Host "ERRO: O arquivo 'db-explorer-mcp.exe' não foi encontrado em: $scriptDir" -ForegroundColor Red
    exit 1
}

# 2. Perguntar onde deseja manter os binários
$defaultBinDir = Join-Path $env:USERPROFILE ".local\bin"
Write-Host ""
Write-Host "Onde você deseja instalar os binários permanentemente?" -ForegroundColor White
Write-Host "  [1] Copiar para '$defaultBinDir' (Recomendado - evita perda acidental)" -ForegroundColor Green
Write-Host "  [2] Usar a pasta atual ('$scriptDir')" -ForegroundColor White
$choice = Read-Host "Escolha uma opção [Padrão: 1]"

$serverExePath = $localServerExe
$managerExePath = $localManagerExe

if ($choice -ne "2") {
    New-Item -ItemType Directory -Force -Path $defaultBinDir | Out-Null
    $serverExePath = Join-Path $defaultBinDir "db-explorer-mcp.exe"
    $managerExePath = Join-Path $defaultBinDir "db-explorer-manager.exe"

    Copy-Item -Path $localServerExe -Destination $serverExePath -Force
    if (Test-Path $localManagerExe) {
        Copy-Item -Path $localManagerExe -Destination $managerExePath -Force
    }

    Write-Host "✅ Binários copiados para: $defaultBinDir" -ForegroundColor Green

    # Verificar PATH
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (-not ($userPath -split ';' -contains $defaultBinDir)) {
        Write-Host "⚠️ AVISO: A pasta '$defaultBinDir' ainda não está no seu PATH de usuário." -ForegroundColor Yellow
        $addPath = Read-Host "Deseja adicionar ao PATH do usuário automaticamente agora? (s/n) [Padrão: s]"
        if ($addPath -ne "n" -and $addPath -ne "N") {
            $newPath = "$userPath;$defaultBinDir"
            [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
            $env:Path = "$env:Path;$defaultBinDir"
            Write-Host "✅ Pasta adicionada ao PATH do usuário com sucesso!" -ForegroundColor Green
        }
    }
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
Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host " Configuração dos Clientes de IA / MCP" -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan

# 1. Claude Code
Write-Host ""
Write-Host "--- 1. Claude Code (CLI) ---" -ForegroundColor Cyan
if (Prompt-YesNo "Deseja registrar o MCP no Claude Code?") {
    Write-Host "Escolha o escopo de ativação no Claude Code:" -ForegroundColor White
    Write-Host "  [1] Global / User (Disponível em qualquer projeto/pasta) [Recomendado]" -ForegroundColor Green
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
        Write-Host "⚠️ O comando 'claude' não foi encontrado no PATH atual." -ForegroundColor Yellow
        Write-Host "Após instalar o Claude Code CLI, você pode registrar o MCP executando:" -ForegroundColor White
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
Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "✨ Concluído!" -ForegroundColor Cyan
Write-Host "1. Para adicionar ou testar suas conexões com bancos:" -ForegroundColor White
Write-Host "     db-explorer-manager (ou `"$managerExePath`")" -ForegroundColor Yellow
Write-Host "2. Para verificar servidores ativos no Claude Code:" -ForegroundColor White
Write-Host "     claude mcp list" -ForegroundColor Yellow
Write-Host "==========================================================" -ForegroundColor Cyan
