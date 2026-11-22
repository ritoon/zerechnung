# ZeRechnung server

`zerechnung-server` is the HTTP API of ZeRechnung, packaged as a container image for microservices. One request turns invoice data into a signed Factur-X PDF: the server renders the invoice as a PDF/A-3 document, embeds the CII XML, validates the result and signs it (PAdES). It also validates, signs, verifies, extracts and converts the e-invoices you send or receive.

- One image for Linux (amd64, arm64, arm/v7) and Windows Server (Nano Server ltsc2022 and ltsc2025), on Docker Hub as [`ritoon/zerechnung`](https://hub.docker.com/r/ritoon/zerechnung).
- Authentication by signed JWT (HMAC, RSA, RSA-PSS, ECDSA, EdDSA, JWKS) or basic auth.
- Signing keys from PEM files or PKCS#12 (`.p12`, `.pfx`), given as environment variables or Docker/Kubernetes secrets.
- Logs in JSON to stdout, a file, syslog, GELF (Graylog), OpenTelemetry (OTLP/HTTP), Grafana Loki or any HTTP collector, several at once.
- Health and readiness probes, Prometheus metrics, request IDs, W3C trace context, graceful shutdown.
- Static binary, standard library only, ~25 MB image, runs as a non-root user on a read-only file system.

The server is a Go module of its own (`server/go.mod`): it calls the network (JWKS, log outputs), which the `zerechnung` package never does.

## Quick start

```bash
docker run -d --name zerechnung -p 8080:8080 \
  -e ZR_BASIC_USERS='billing:change-me' \
  -e ZR_SIGN_PKCS12_FILE=/run/secrets/sign.p12 \
  -e ZR_SIGN_PKCS12_PASSWORD='p12 password' \
  -v "$PWD/company.p12:/run/secrets/sign.p12:ro" \
  ritoon/zerechnung
```

Create a signed Factur-X invoice from JSON ([examples/invoice-request.json](examples/invoice-request.json)):

```bash
curl -u billing:change-me -H 'Content-Type: application/json' \
  --data-binary @server/examples/invoice-request.json \
  -o FA-2026-00042.pdf http://localhost:8080/v1/invoices
```

The response is the PDF: rendered in French, with the CII XML embedded, validated against EN 16931 and the French rules, and signed. The headers say what was made:

```
HTTP/1.1 201 Created
Content-Type: application/pdf
Content-Disposition: attachment; filename="FA-2026-00042.pdf"
X-Invoice-Number: FA-2026-00042
X-Invoice-Profile: EN 16931
X-Invoice-Signed: true
Repr-Digest: sha-256=:...:
X-Request-ID: 3f2a...
```

Without a signing key, the server creates unsigned invoices and answers `501` to the requests that ask for a signature. With `ZR_AUTH=none`, it accepts anonymous requests: for a local trial only.

## Images

| Tag | Content |
| --- | --- |
| `latest`, `X.Y.Z`, `X.Y` | The releases: Linux and Windows under one tag |
| `edge` | The `main` branch |
| `X.Y.Z-linux` | Linux only (amd64, arm64, arm/v7) |
| `X.Y.Z-nanoserver-ltsc2022`, `X.Y.Z-nanoserver-ltsc2025` | Windows only |

Docker picks the image of the host:

- **Linux**, and **Docker Desktop on macOS and Windows** (Linux containers): the Linux image, amd64 or arm64 (Apple silicon).
- **Windows Server 2022 and 2025**, and Windows 10/11 in Windows containers mode: the Nano Server image whose version matches the host (process isolation needs the same build of Windows; Hyper-V isolation runs the ltsc2022 image on newer hosts too).

On Windows containers, mount directories rather than files, and use Windows paths:

```powershell
docker run -d -p 8080:8080 `
  -e ZR_BASIC_USERS=billing:change-me `
  -e ZR_SIGN_PKCS12_FILE=C:\secrets\company.p12 `
  -e ZR_SIGN_PKCS12_PASSWORD="p12 password" `
  -v C:\zerechnung\secrets:C:\secrets:ro `
  ritoon/zerechnung
```

The images hold two programs: `zerechnung-server` (the entrypoint) and the [`zerechnung` command](../README.md#command-line). `docker run --rm ritoon/zerechnung version` prints the version.

### Without Docker

Each [release](https://github.com/ritoon/zerechnung/releases) carries both programs for Windows (amd64, arm64), macOS (amd64, arm64) and Linux (amd64, arm64, armv7). They read the same environment variables:

```powershell
$env:ZR_BASIC_USERS = "billing:change-me"
$env:ZR_SIGN_PKCS12_FILE = "C:\zerechnung\company.p12"
$env:ZR_SIGN_PKCS12_PASSWORD = "p12 password"
.\zerechnung-server.exe
```

To build the images yourself, from the root of the repository:

```bash
docker build -f server/Dockerfile -t zerechnung .
docker build -f server/Dockerfile.windows --build-arg WINDOWS_VERSION=ltsc2022 -t zerechnung .   # on Windows
```

## API

The OpenAPI 3.1 description is served on `GET /openapi.json`, with the schema of the invoice. Every endpoint under `/v1` needs credentials.

| Endpoint | Body | Response |
| --- | --- | --- |
| `POST /v1/invoices` | JSON request, or multipart form | The Factur-X PDF (or JSON with `Accept: application/json`) |
| `POST /v1/xml` | JSON: `invoice`, `format` | The XML invoice: CII, UBL, FatturaPA or Facturae |
| `POST /v1/validate` | PDF or XML | Validation report |
| `POST /v1/sign` | PDF, or FatturaPA XML | The signed PDF (PAdES), or the `.p7m` (CAdES) / XAdES file |
| `POST /v1/verify` | Signed PDF, `.p7m` or XAdES file | The signatures and their integrity |
| `POST /v1/extract` | Factur-X PDF; XML with `?format=json` | The embedded XML, or the invoice as JSON |
| `POST /v1/pdfa` | PDF | The PDF converted to PDF/A-3, or the reasons of the refusal |
| `GET /v1/info` | | Version, signing certificate, formats, profiles, rule sets |
| `GET /healthz`, `GET /readyz` | | Liveness and readiness probes, without credentials |
| `GET /metrics` | | Prometheus metrics, without credentials |

The documents (PDF, XML) are sent as the body, with their content type, or as the part `file` of a multipart form.

### Create an invoice

`POST /v1/invoices` takes a JSON object:

| Field | |
| --- | --- |
| `invoice` | The invoice, in the JSON form of [`invoice.Invoice`](https://pkg.go.dev/github.com/ritoon/zerechnung/invoice#Invoice) (the output of `zerechnung json`, and of `/v1/extract?format=json`) |
| `xml` | Instead of `invoice`: a CII invoice made elsewhere (base64), embedded as is |
| `pdf` | Your own PDF (base64) to turn into a Factur-X invoice; when absent, the server renders the invoice |
| `language` | Language of the rendered PDF: `en`, `fr`, `de`, `it`, `es`, `pt`, `nl`; when absent, the invoice's `Language` or the seller's country, English otherwise |
| `profile` | Overrides the profile: `MINIMUM`, `BASIC WL`, `BASIC`, `EN 16931`, `EXTENDED`, `EXTENDED-CTC-FR`, `XRECHNUNG`, `PEPPOL`, `NLCIUS`, `CIUS-IT` |
| `calculate` | Computes the line amounts, totals and VAT breakdown; `true` by default |
| `validation` | `{"rules": ["fr", "de", "choruspro", "peppol", "nl", "it"], "auto": true, "skip": false, "profile": "", "reject_basicwl": false, "service_code_required": false, "engagement_required": false}`; EN 16931 and the Factur-X rules always apply, `auto` adds the rules of the seller's country |
| `convert_pdfa`, `require_pdfa` | For your own PDF: convert it to PDF/A-3, or refuse it unless it declares PDF/A |
| `sign` | Signs the PDF; `ZR_SIGN_DEFAULT` (true) when absent |
| `reason`, `location` | Of the signature, instead of `ZR_SIGN_REASON` and `ZR_SIGN_LOCATION` |
| `attachments` | Supporting documents: `[{"name": "timesheet.csv", "mime_type": "text/csv", "description": "", "data": "<base64>"}]` |
| `filename` | Name of the PDF returned; from the invoice number when absent |

An invoice with validation errors is refused with `422` and the report. With `Accept: application/json`, the response is a JSON object holding the PDF in base64, its SHA-256, the profile and the validation report.

To send your own PDF without base64, use a multipart form with the parts `request` (the JSON options above), `invoice`, `xml`, `pdf` and `attachment` (one part per file):

```bash
curl -u billing:change-me \
  -F 'request={"convert_pdfa": true};type=application/json' \
  -F 'invoice=@invoice.json;type=application/json' \
  -F 'pdf=@my-invoice.pdf;type=application/pdf' \
  -F 'attachment=@timesheet.csv;type=text/csv' \
  -o factur-x.pdf http://localhost:8080/v1/invoices
```

### Other operations

```bash
# Validate a received invoice (PDF or XML) with the German rules
curl -u billing:change-me -H 'Content-Type: application/pdf' \
  --data-binary @invoice.pdf 'http://localhost:8080/v1/validate?rules=de'

# Sign an existing PDF, and verify it
curl -u billing:change-me -H 'Content-Type: application/pdf' \
  --data-binary @invoice.pdf -o signed.pdf 'http://localhost:8080/v1/sign?reason=Archive'
curl -u billing:change-me -H 'Content-Type: application/pdf' \
  --data-binary @signed.pdf http://localhost:8080/v1/verify

# Read a Factur-X invoice as JSON, and write it as UBL
curl -u billing:change-me -H 'Content-Type: application/pdf' \
  --data-binary @invoice.pdf -o invoice.json 'http://localhost:8080/v1/extract?format=json'
curl -u billing:change-me -H 'Content-Type: application/json' \
  -d "{\"format\": \"ubl\", \"invoice\": $(cat invoice.json)}" http://localhost:8080/v1/xml

# Sign a FatturaPA invoice for the SDI (CAdES .p7m; ?format=xades for XAdES)
curl -u billing:change-me -H 'Content-Type: application/xml' \
  --data-binary @IT01234567890_00001.xml -o IT01234567890_00001.xml.p7m http://localhost:8080/v1/sign
```

`/v1/validate` takes the validation options as query parameters: `rules=fr,de`, `auto`, `profile`, `reject_basicwl`, `service_code_required`, `engagement_required`. `/v1/xml` takes them in the `validation` field, as `/v1/invoices` does.

### Errors

Errors are [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) problem details (`application/problem+json`) with a stable `code` and the request ID:

```json
{
  "type": "about:blank",
  "title": "the invoice is not valid",
  "status": 422,
  "detail": "1 error(s), first: error [BR-FR] BR-FR-13_BT-34: ...",
  "code": "invalid_invoice",
  "request_id": "c4ca948a80b30cf0cda8d393",
  "report": {"valid": false, "errors": 1, "findings": [...]}
}
```

| Status | Code |
| --- | --- |
| 400 | `bad_request`: malformed JSON, unknown field, bad query parameter |
| 401, 403 | Missing or invalid credentials; `403` for a valid token that lacks a required scope |
| 413 | `too_large`: the body exceeds `ZR_MAX_BODY_BYTES` |
| 422 | `invalid_invoice`, `not_convertible`, `not_signable`, `no_invoice`, `unsupported_document`, `bad_request` (an invalid value) |
| 501 | `signing_not_configured`: no signing key |
| 503 | `certificate_not_valid`: the signing certificate is expired or not yet valid; and `/readyz` during the shutdown |

## Authentication

The methods are chosen by `ZR_AUTH` (`jwt`, `basic`, both separated by a comma, or `none`), or from the variables set: a JWT key enables `jwt`, `ZR_BASIC_USERS` enables `basic`. The server refuses to start without any method unless `ZR_AUTH=none`.

### JWT

`Authorization: Bearer <token>`. The token is a JWS in compact form, signed with:

- **HMAC** (HS256, HS384, HS512): `ZR_JWT_SECRET`, at least 32 bytes.
- **Public keys** (RS256/384/512, PS256/384/512, ES256/384/512, EdDSA): `ZR_JWT_PUBLIC_KEYS`, PEM public keys or certificates, several allowed; or `ZR_JWT_JWKS_URL`, the JWKS of your identity provider (Keycloak, Auth0, Entra ID, Okta, Dex...), fetched at start, refreshed every `ZR_JWT_JWKS_REFRESH` and when a token names an unknown `kid`.

The algorithm of a token must match the type of the key (no `alg: none`, no HMAC with a public key); `ZR_JWT_ALGORITHMS` narrows the list. The claims checked are `exp` (required), `nbf` and `iat` with `ZR_JWT_LEEWAY`, `iss` against `ZR_JWT_ISSUER` and `aud` against `ZR_JWT_AUDIENCE` when set, and the scopes of `ZR_JWT_REQUIRED_SCOPES` in `scope` or `scp`. The subject (`sub`) appears in the access log.

A token for a test, with HS256:

```bash
secret='a-secret-of-at-least-32-bytes-long!!'
b64() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }
header=$(printf '{"alg":"HS256","typ":"JWT"}' | b64)
payload=$(printf '{"sub":"billing-service","exp":%d}' $(( $(date +%s) + 3600 )) | b64)
sig=$(printf '%s.%s' "$header" "$payload" | openssl dgst -sha256 -hmac "$secret" -binary | b64)
curl -H "Authorization: Bearer $header.$payload.$sig" http://localhost:8080/v1/info
```

### Basic auth

`ZR_BASIC_USERS` lists `user:password` entries separated by commas or line breaks (lines starting with `#` are comments). Store the SHA-256 of the password rather than the password itself: `user:sha256:<hex>`, computed with `printf %s 'password' | sha256sum`. The comparison takes constant time. Use basic auth behind TLS only: set `ZR_TLS_CERT_FILE` and `ZR_TLS_KEY_FILE`, or terminate TLS in front of the server.

## Signing

The server signs with one key and its certificate chain:

| Variables | Key |
| --- | --- |
| `ZR_SIGN_PKCS12` (base64) or `ZR_SIGN_PKCS12_FILE`, and `ZR_SIGN_PKCS12_PASSWORD` | PKCS#12 file (`.p12`, `.pfx`), as exported by Windows, macOS, OpenSSL 1 and 3, or your certification authority |
| `ZR_SIGN_KEY` or `ZR_SIGN_KEY_FILE`, `ZR_SIGN_KEY_PASSWORD`, and `ZR_SIGN_CERT` or `ZR_SIGN_CERT_FILE` | PEM private key (PKCS#1, SEC 1, PKCS#8, encrypted PKCS#8) and PEM certificates: the signing certificate, then the intermediate ones |

PEM values may be base64-encoded to fit in one line. Keys are RSA (2048 bits or more) or ECDSA (P-256, P-384, P-521).

PDFs are signed in PAdES B-B (`ETSI.CAdES.detached`), as an incremental update that keeps the PDF/A-3 conformance of the invoice, with `ZR_SIGN_HASH` (sha256, sha384, sha512), RSA-PSS with `ZR_SIGN_RSA_PSS=true`, and the reason, location, contact and name of `ZR_SIGN_REASON`, `ZR_SIGN_LOCATION`, `ZR_SIGN_CONTACT` and `ZR_SIGN_NAME`. FatturaPA files are signed in CAdES-BES or XAdES-BES, as the SDI requires.

At start, the server logs the certificate's subject and expiry, warns 30 days before the expiry, and refuses to sign with an expired certificate (`503`). `GET /v1/info` reports the certificate.

## Logs

The server writes one JSON record per request (`request_id`, `method`, `route`, `status`, `duration_ms`, `bytes_in`, `bytes_out`, `auth_method`, `subject`, `trace_id`), plus the start, stop and error events. `ZR_LOG_OUTPUTS` lists the outputs, separated by commas:

| Output | Settings |
| --- | --- |
| `stdout` (default), `stderr` | For `docker logs`, Kubernetes, Fluent Bit, Vector, Datadog or Promtail agents |
| `file` | `ZR_LOG_FILE`, rotated at `ZR_LOG_FILE_MAX_BYTES` (100 MiB), `ZR_LOG_FILE_MAX_BACKUPS` (5) kept |
| `syslog` | `ZR_LOG_SYSLOG_ADDRESS`: `udp://host:514`, `tcp://host:514` or `tls://host:6514` (RFC 5424); `ZR_LOG_SYSLOG_FACILITY` (local0) |
| `gelf` | `ZR_LOG_GELF_ADDRESS`: `udp://graylog:12201` (chunked) or `tcp://graylog:12201` |
| `otlp` | `ZR_LOG_OTLP_ENDPOINT` (`http://collector:4318/v1/logs`), `ZR_LOG_OTLP_HEADERS`; or the standard `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES` |
| `loki` | `ZR_LOG_LOKI_URL` (`http://loki:3100`), `ZR_LOG_LOKI_LABELS` (`env=prod,team=billing`), `ZR_LOG_LOKI_TENANT` |
| `http` | `ZR_LOG_HTTP_URL`, `ZR_LOG_HTTP_HEADERS`, `ZR_LOG_HTTP_FORMAT` (`ndjson` or `json` array): Elasticsearch/OpenSearch ingest, Splunk HEC behind a proxy, Logstash, Vector, Fluentd HTTP input |

`ZR_LOG_LEVEL` (debug, info, warn, error) and `ZR_LOG_FORMAT` (json, text) apply to every output. The network outputs send in batches (`ZR_LOG_BATCH_SIZE`, `ZR_LOG_FLUSH_INTERVAL`) from a buffer of `ZR_LOG_BUFFER` records, retry three times, and never slow the requests down: when the collector is unreachable and the buffer is full, records are dropped and counted in `zerechnung_log_records_total{result="dropped"}`. The buffers are flushed at shutdown. Headers are `key=value` pairs separated by commas, percent-encoded as in the OpenTelemetry variables.

## Observability

- `GET /healthz` answers `200` while the process runs; `GET /readyz` answers `503` until the JWKS is loaded and once the shutdown starts.
- `GET /metrics` (Prometheus text format; `ZR_METRICS=false` to disable): `zerechnung_http_requests_total`, `zerechnung_http_request_duration_seconds`, `zerechnung_http_requests_in_flight`, `zerechnung_operations_total{operation,result}`, `zerechnung_signatures_total{format}`, `zerechnung_log_records_total{output,result}`, `zerechnung_build_info`, `process_start_time_seconds`, `go_goroutines`, `go_memstats_heap_alloc_bytes`.
- Every response carries `X-Request-ID`, taken from the request when the caller sends one. The `trace_id` of a W3C `traceparent` header goes into the logs.
- On `SIGTERM` (and `docker stop` on Windows), the server fails its readiness probe, finishes the requests in flight within `ZR_SHUTDOWN_TIMEOUT` and flushes the logs.
- The image's `HEALTHCHECK` runs `zerechnung-server healthcheck`, as there is no shell nor curl in it.

## Configuration

Every variable starting with `ZR_` may instead be read from a file with the suffix `_FILE`, for Docker and Kubernetes secrets: `ZR_JWT_SECRET_FILE=/run/secrets/jwt`. The server reports every invalid variable at start, then exits.

| Variable | Default | |
| --- | --- | --- |
| `ZR_PORT` (or `PORT`) | `8080` | Listening port |
| `ZR_HOST` | all interfaces | Listening address |
| `ZR_ADDR` | | `host:port`, instead of `ZR_HOST` and `ZR_PORT` |
| `ZR_TLS_CERT_FILE`, `ZR_TLS_KEY_FILE` | | Serve HTTPS |
| `ZR_MAX_BODY_BYTES` | `32MiB` | Largest request body (`10MB`, `64MiB`) |
| `ZR_READ_TIMEOUT`, `ZR_WRITE_TIMEOUT`, `ZR_IDLE_TIMEOUT` | `1m`, `2m`, `2m` | HTTP timeouts |
| `ZR_SHUTDOWN_TIMEOUT` | `20s` | Time given to the requests in flight |
| `ZR_SERVICE_NAME` (or `OTEL_SERVICE_NAME`) | `zerechnung` | Service name in the logs |
| `ZR_METRICS` | `true` | Serve `/metrics` |
| `ZR_CA_FILE` | | Extra CA certificates (PEM) for the JWKS and the log collectors |
| `ZR_AUTH` | from the keys | `jwt`, `basic`, `jwt,basic` or `none` |
| `ZR_AUTH_REALM` | `zerechnung` | Realm of the `WWW-Authenticate` challenge |
| `ZR_BASIC_USERS` | | `user:password` or `user:sha256:<hex>` entries |
| `ZR_JWT_SECRET` | | HMAC key, 32 bytes or more |
| `ZR_JWT_PUBLIC_KEYS` | | PEM public keys or certificates |
| `ZR_JWT_JWKS_URL`, `ZR_JWT_JWKS_REFRESH` | `15m` | JWKS of the identity provider |
| `ZR_JWT_ISSUER`, `ZR_JWT_AUDIENCE` | | Accepted `iss` and `aud`, separated by commas |
| `ZR_JWT_ALGORITHMS` | all | Accepted algorithms |
| `ZR_JWT_LEEWAY` | `1m` | Clock skew allowed |
| `ZR_JWT_REQUIRED_SCOPES` | | Scopes every token must hold |
| `ZR_SIGN_PKCS12`, `ZR_SIGN_PKCS12_PASSWORD` | | PKCS#12 signing key |
| `ZR_SIGN_KEY`, `ZR_SIGN_KEY_PASSWORD`, `ZR_SIGN_CERT` | | PEM signing key and certificates |
| `ZR_SIGN_HASH` | `sha256` | `sha256`, `sha384`, `sha512` |
| `ZR_SIGN_RSA_PSS` | `false` | RSA-PSS instead of PKCS#1 v1.5 |
| `ZR_SIGN_REASON`, `ZR_SIGN_LOCATION`, `ZR_SIGN_CONTACT`, `ZR_SIGN_NAME` | | Signature dictionary |
| `ZR_SIGN_DEFAULT` | `true` | Sign the created invoices unless the request says otherwise |
| `ZR_LOG_*` | | See [Logs](#logs) |

## Deployment

### Docker Compose

[examples/compose.yaml](examples/compose.yaml) runs the server with its secrets, a read-only file system and its logs sent to Loki:

```bash
docker compose -f server/examples/compose.yaml up -d
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: zerechnung
spec:
  replicas: 2
  selector:
    matchLabels: {app: zerechnung}
  template:
    metadata:
      labels: {app: zerechnung}
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
    spec:
      securityContext:
        runAsNonRoot: true
        seccompProfile: {type: RuntimeDefault}
      containers:
        - name: zerechnung
          image: ritoon/zerechnung:latest
          ports: [{containerPort: 8080, name: http}]
          env:
            - {name: ZR_JWT_JWKS_URL, value: "https://sso.example.com/realms/billing/protocol/openid-connect/certs"}
            - {name: ZR_JWT_ISSUER, value: "https://sso.example.com/realms/billing"}
            - {name: ZR_JWT_AUDIENCE, value: "zerechnung"}
            - {name: ZR_SIGN_PKCS12_FILE, value: /secrets/sign.p12}
            - {name: ZR_SIGN_PKCS12_PASSWORD_FILE, value: /secrets/sign.password}
            - {name: ZR_LOG_OUTPUTS, value: "stdout,otlp"}
            - {name: OTEL_EXPORTER_OTLP_ENDPOINT, value: "http://otel-collector.observability:4318"}
          volumeMounts: [{name: secrets, mountPath: /secrets, readOnly: true}]
          readinessProbe: {httpGet: {path: /readyz, port: http}}
          livenessProbe: {httpGet: {path: /healthz, port: http}}
          resources:
            requests: {cpu: 100m, memory: 64Mi}
            limits: {memory: 512Mi}
          securityContext:
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities: {drop: [ALL]}
      terminationGracePeriodSeconds: 30
      volumes:
        - name: secrets
          secret: {secretName: zerechnung-signing}
```

The server keeps no state: scale it horizontally. Requests are processed in memory, without temporary files; size the memory limit for `ZR_MAX_BODY_BYTES` times the concurrent requests.

## Development

```bash
cd server
go test -race ./...
go run ./cmd/zerechnung-server          # with ZR_AUTH=none for a local trial
scripts/smoke.sh zerechnung:test        # end-to-end test of a built image
```

The test certificates of `internal/keys/testdata` are fictitious; see the [README](internal/keys/testdata/README.md) there.
