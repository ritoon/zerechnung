# ZeRechnung server

HTTP API for e-invoices, for microservices: one request turns invoice data into a **signed Factur-X PDF** (PDF/A-3 rendering, CII XML, validation, PAdES signature). It also validates, signs, verifies, extracts and converts Factur-X, ZUGFeRD, XRechnung, UBL, Peppol, FatturaPA and Facturae invoices.

- **Linux** (amd64, arm64, arm/v7) and **Windows Server** (Nano Server ltsc2022, ltsc2025) under one tag; runs on Docker Desktop for macOS and Windows.
- **Authentication**: signed JWT (HS*, RS*, PS*, ES*, EdDSA, JWKS of your identity provider) or basic auth.
- **Signing keys** from PKCS#12 (`.p12`, `.pfx`) or PEM, as environment variables or Docker/Kubernetes secrets (`*_FILE`).
- **Logs** to stdout, file, syslog, GELF (Graylog), OpenTelemetry (OTLP/HTTP), Grafana Loki or any HTTP collector.
- **Operations**: `/healthz`, `/readyz`, Prometheus `/metrics`, `X-Request-ID`, W3C trace context, graceful shutdown, OpenAPI 3.1 on `/openapi.json`.
- ~25 MB, static binary, non-root user, read-only file system, no temporary files.

## Quick start

```bash
docker run -d -p 8080:8080 \
  -e ZR_BASIC_USERS='billing:change-me' \
  -e ZR_SIGN_PKCS12_FILE=/run/secrets/sign.p12 \
  -e ZR_SIGN_PKCS12_PASSWORD='p12 password' \
  -v "$PWD/company.p12:/run/secrets/sign.p12:ro" \
  ritoon/zerechnung
```

```bash
curl -u billing:change-me -H 'Content-Type: application/json' \
  --data-binary @invoice-request.json -o invoice.pdf \
  http://localhost:8080/v1/invoices
```

[invoice-request.json](https://github.com/ritoon/zerechnung/blob/main/server/examples/invoice-request.json) is a French invoice; the response is the signed Factur-X PDF.

## Endpoints

| Endpoint | |
| --- | --- |
| `POST /v1/invoices` | Invoice JSON (or your PDF, or a CII XML) → signed Factur-X PDF |
| `POST /v1/xml` | Invoice JSON → CII, UBL, FatturaPA or Facturae |
| `POST /v1/validate` | PDF or XML → validation report (EN 16931, French, German, Dutch, Italian, Peppol, Chorus Pro rules) |
| `POST /v1/sign`, `POST /v1/verify` | PAdES for PDFs; CAdES and XAdES for FatturaPA |
| `POST /v1/extract` | Factur-X PDF → XML or JSON |
| `POST /v1/pdfa` | PDF → PDF/A-3 |

## Tags

| Tag | |
| --- | --- |
| `latest`, `X.Y.Z`, `X.Y` | Releases, Linux and Windows |
| `edge` | The `main` branch |
| `X.Y.Z-linux`, `X.Y.Z-nanoserver-ltsc2022`, `X.Y.Z-nanoserver-ltsc2025` | One OS only |

## Main settings

| Variable | |
| --- | --- |
| `ZR_PORT` | Listening port, `8080` |
| `ZR_BASIC_USERS` | `user:password` or `user:sha256:<hex>`, separated by commas |
| `ZR_JWT_SECRET`, `ZR_JWT_PUBLIC_KEYS`, `ZR_JWT_JWKS_URL` | JWT keys; `ZR_JWT_ISSUER`, `ZR_JWT_AUDIENCE`, `ZR_JWT_REQUIRED_SCOPES` |
| `ZR_SIGN_PKCS12_FILE`, `ZR_SIGN_PKCS12_PASSWORD` | PKCS#12 signing key; or `ZR_SIGN_KEY_FILE` and `ZR_SIGN_CERT_FILE` (PEM) |
| `ZR_SIGN_HASH`, `ZR_SIGN_REASON`, `ZR_SIGN_LOCATION` | Signature settings |
| `ZR_LOG_OUTPUTS` | `stdout`, `file`, `syslog`, `gelf`, `otlp`, `loki`, `http`, several separated by commas |

Every variable can be read from a file with the suffix `_FILE`. The full reference, the Compose and Kubernetes examples and the Windows notes are in the [documentation](https://github.com/ritoon/zerechnung/tree/main/server#readme).

Source: [github.com/ritoon/zerechnung](https://github.com/ritoon/zerechnung) · MIT license
