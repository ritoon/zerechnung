# Smoke test of a Windows image of zerechnung-server: it starts the
# container with basic auth and the fictitious test certificate, creates a
# signed invoice from examples\invoice-request.json and verifies it.
#
#	server\scripts\smoke.ps1 zerechnung:test
param([Parameter(Mandatory = $true)][string]$Image)

$ErrorActionPreference = 'Stop'
$server = Split-Path -Parent $PSScriptRoot
$name = "zerechnung-smoke-$PID"
$out = Join-Path ([IO.Path]::GetTempPath()) $name
New-Item -ItemType Directory -Force $out | Out-Null

function Fail([string]$msg) {
	Write-Host "smoke: $msg"
	docker logs $name
	docker rm -f $name | Out-Null
	exit 1
}

# Windows containers mount directories, not single files.
docker run -d --name $name `
	-e ZR_BASIC_USERS=smoke:smoke-password `
	-e 'ZR_SIGN_PKCS12_FILE=C:\secrets\modern.p12' `
	-e ZR_SIGN_PKCS12_PASSWORD=test1234 `
	-e 'ZR_SIGN_REASON=Smoke test' `
	-v "$server\internal\keys\testdata:C:\secrets:ro" `
	$Image | Out-Null
if ($LASTEXITCODE -ne 0) { Fail 'docker run failed' }

# The container is reached on its own address: the loopback forwarding of
# published ports depends on the version of Windows.
$ip = docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' $name
$url = "http://${ip}:8080"
$ready = $false
for ($i = 0; $i -lt 60; $i++) {
	curl.exe -fsS "$url/readyz" 2>$null | Out-Null
	if ($LASTEXITCODE -eq 0) { $ready = $true; break }
	Start-Sleep -Milliseconds 500
}
if (-not $ready) { Fail 'not ready' }

$code = curl.exe -s -o NUL -w '%{http_code}' "$url/v1/info"
if ($code -ne '401') { Fail "GET /v1/info without credentials: $code, want 401" }

$pdf = Join-Path $out 'invoice.pdf'
$headers = Join-Path $out 'headers.txt'
$code = curl.exe -s -u smoke:smoke-password -H 'Content-Type: application/json' `
	--data-binary "@$server\examples\invoice-request.json" -D $headers -o $pdf -w '%{http_code}' "$url/v1/invoices"
if ($code -ne '201') { Fail "POST /v1/invoices: $code $(Get-Content -Raw $pdf)" }
if (-not (Select-String -Quiet -Path $headers -Pattern '^x-invoice-signed: true')) { Fail 'the invoice is not signed' }

$verify = curl.exe -fsS -u smoke:smoke-password -H 'Content-Type: application/pdf' --data-binary "@$pdf" "$url/v1/verify" | Out-String
if ($verify -notmatch '"valid": ?true') { Fail "POST /v1/verify: $verify" }

$health = docker inspect -f '{{.State.Health.Status}}' $name
docker stop -t 20 $name | Out-Null
docker logs $name
docker rm -f $name | Out-Null
Write-Host "smoke: OK ($((Get-Item $pdf).Length) bytes, signed and valid; health: $health)"
