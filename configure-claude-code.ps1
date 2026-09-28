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

# 3. Escolher o escopo no Claude Code
Write-Host ""
Write-Host "Escolha o escopo de ativação no Claude Code:" -ForegroundColor White
Write-Host "  [1] Global / User (Disponível em qualquer projeto/pasta) [Recomendado]" -ForegroundColor Green
Write-Host "  [2] Local (Apenas para a pasta atual)" -ForegroundColor White
$scopeChoice = Read-Host "Escolha o escopo [Padrão: 1]"

$scope = if ($scopeChoice -eq "2") { "local" } else { "user" }

# 4. Registrar no Claude Code CLI
Write-Host ""
if (Get-Command "claude" -ErrorAction SilentlyContinue) {
    Write-Host ">> Registrando MCP 'db-explorer' no Claude Code com escopo '$scope'..." -ForegroundColor Green
    
    # Remover registro anterior se existir para evitar conflito
    try {
        & claude mcp remove --scope $scope db-explorer 2>$null | Out-Null
    } catch { }

    # Executar claude mcp add
    & claude mcp add --scope $scope db-explorer -- "$serverExePath"

    if ($LASTEXITCODE -eq 0) {
        Write-Host ""
        Write-Host "✅ MCP 'db-explorer' registrado e configurado com sucesso no Claude Code!" -ForegroundColor Green
    } else {
        Write-Host ""
        Write-Host "⚠️ Ocorreu um aviso durante a execução do claude mcp add." -ForegroundColor Yellow
        Write-Host "Você pode tentar registrar manualmente executando:" -ForegroundColor White
        Write-Host "  claude mcp add --scope $scope db-explorer -- `"$serverExePath`"" -ForegroundColor Cyan
    }
} else {
    Write-Host "⚠️ O comando 'claude' não foi encontrado no PATH atual." -ForegroundColor Yellow
    Write-Host "Após instalar o Claude Code CLI, você pode registrar o MCP executando:" -ForegroundColor White
    Write-Host "  claude mcp add --scope $scope db-explorer -- `"$serverExePath`"" -ForegroundColor Cyan
}

Write-Host ""
Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "✨ Próximos Passos:" -ForegroundColor White
Write-Host "1. Para adicionar ou testar suas conexões com bancos:" -ForegroundColor White
Write-Host "     db-explorer-manager (ou `"$managerExePath`")" -ForegroundColor Yellow
Write-Host "2. Para verificar os servidores ativos no Claude Code:" -ForegroundColor White
Write-Host "     claude mcp list" -ForegroundColor Yellow
Write-Host "==========================================================" -ForegroundColor Cyan
