#!/usr/bin/env bash
# Smoke test of a Linux image of zerechnung-server: it starts the container
# with basic auth and the fictitious test certificate, creates a signed
# invoice from examples/invoice-request.json, verifies it and stops the
# container.
#
#	server/scripts/smoke.sh zerechnung:test
set -euo pipefail

image=${1:?usage: smoke.sh IMAGE}
here=$(cd "$(dirname "$0")/.." && pwd)
port=${SMOKE_PORT:-18080}
name=zerechnung-smoke-$$
out=$(mktemp -d)
trap 'docker rm -f "$name" >/dev/null 2>&1 || true; rm -rf "$out"' EXIT

fail() {
	echo "smoke: $*" >&2
	docker logs "$name" >&2 || true
	exit 1
}

docker run -d --name "$name" --read-only -p "127.0.0.1:$port:8080" \
	-e ZR_BASIC_USERS=smoke:smoke-password \
	-e ZR_SIGN_PKCS12_FILE=/run/secrets/sign.p12 \
	-e ZR_SIGN_PKCS12_PASSWORD=test1234 \
	-e ZR_SIGN_REASON="Smoke test" \
	-v "$here/internal/keys/testdata/modern.p12:/run/secrets/sign.p12:ro" \
	"$image" >/dev/null

url=http://127.0.0.1:$port
for _ in $(seq 60); do
	curl -fsS "$url/readyz" >/dev/null 2>&1 && break
	sleep 0.5
done
curl -fsS "$url/readyz" >/dev/null || fail "not ready"

code=$(curl -s -o /dev/null -w '%{http_code}' "$url/v1/info")
[ "$code" = 401 ] || fail "GET /v1/info without credentials: $code, want 401"

code=$(curl -s -u smoke:smoke-password -H 'Content-Type: application/json' \
	--data-binary "@$here/examples/invoice-request.json" \
	-D "$out/headers" -o "$out/invoice.pdf" -w '%{http_code}' "$url/v1/invoices")
[ "$code" = 201 ] || fail "POST /v1/invoices: $code: $(cat "$out/invoice.pdf")"
grep -qi '^x-invoice-signed: true' "$out/headers" || fail "the invoice is not signed"
head -c 5 "$out/invoice.pdf" | grep -q '%PDF-' || fail "not a PDF"

verify=$(curl -fsS -u smoke:smoke-password -H 'Content-Type: application/pdf' \
	--data-binary "@$out/invoice.pdf" "$url/v1/verify")
echo "$verify" | grep -Eq '"valid": ?true' || fail "POST /v1/verify: $verify"

report=$(curl -fsS -u smoke:smoke-password -H 'Content-Type: application/pdf' \
	--data-binary "@$out/invoice.pdf" "$url/v1/validate?rules=fr")
echo "$report" | grep -Eq '"valid": ?true' || fail "POST /v1/validate: $report"

# Into a file first: grep -q quits at the first match, and curl, still
# writing to the pipe, would fail (exit 23) under pipefail.
curl -fsS -o "$out/metrics" "$url/metrics" || fail "GET /metrics"
grep -q '^zerechnung_signatures_total' "$out/metrics" || fail "no signature metric"

docker stop -t 20 "$name" >/dev/null
status=$(docker inspect -f '{{.State.ExitCode}}' "$name")
[ "$status" = 0 ] || fail "exit code $status after SIGTERM"
docker logs "$name" 2>&1 | grep -q '"msg":"stopped"' || fail "no graceful shutdown in the logs"

echo "smoke: OK ($(wc -c <"$out/invoice.pdf" | tr -d ' ') bytes, signed and valid)"
