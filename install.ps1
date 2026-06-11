Write-Host "=================================================="
Write-Host " DB Explorer MCP - Instalador"
Write-Host "=================================================="

# Verifica se o bun está instalado
if (Get-Command "bun" -ErrorAction SilentlyContinue) {
    Write-Host "[!] Bun detectado. Executando via Bun..."
    bun install
    bun run src/setup.ts
} else {
    Write-Host "[!] Bun nao detectado. Usando npm/node..."
    if (-not (Get-Command "npm" -ErrorAction SilentlyContinue)) {
        Write-Host "ERRO: Nem Bun nem NPM encontrados. Instale o NodeJS ou o Bun."
        exit 1
    }
    npm install
    npm run build
    node build/setup.js
}
