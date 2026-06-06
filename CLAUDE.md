# CLAUDE.md

Guide Claude Code (claude.ai/code) for work in this repo.

## What this is

Go library. Issue Portuguese (AT-certified) tax documents, export as SAF-T (PT) XML, communicate series/invoices/transport docs to AT webservices. Pure domain + adapters, run by demo/smoke binaries. Evolving milestone-by-milestone into multi-tenant REST API per `ARCHITECTURE_PLAN.md` (Ports & Adapters; M0 restructure done, M1+ pending).

**Git policy: user owns all version control. Never run git commands (commit, branch, etc.) unless explicitly asked.** Where plan says "Checkpoint", stop, let user commit.

## Commands

```bash
go build ./...                                  # build
go test ./...                                   # all tests
go test ./internal/domain -run TestName -v      # single test
go run ./cmd/demo                               # AT certification walkthrough (Apr prologue + §5.1–5.13); writes out/ SAF-T, per-document PDFs, CHECKLIST.txt
go run ./cmd/atsmoke                            # live smoke vs AT *test* webservices; needs AT_NIF/AT_USERNAME/AT_PASSWORD + certs (see cmd/atsmoke/main.go doc comment)
```

No Makefile, no linter config — `gofmt` only. Demo reads `.env` (optional); `AT_SIGNING_KEY_FILE` is required (real RSA signatures, no stub). `PRODUCER_TAX_ID`, `SOFTWARE_NAME`, `PRODUCER_NAME`, `VERSION`, `CERTIFICATE_NUMBER` feed `config.SoftwareIdentity`, validated at boot.

## Architecture

Three layers, strict dependency direction: `config` / `adapter/*` → `domain`. Domain imports nothing from adapters.

### `internal/domain` — pure business logic

- **Document families**, each with own draft type + `Issue*` constructor: sales invoices (FT/FS/FR/NC/ND, `sales_invoice.go`), work documents (`work_document.go`), payments/receipts (RC/RG, `payment.go`), stock movements (`stock_movement.go`). `document_type.go` maps each `DocumentType` to family + rules. `DocumentCore` (`document.go`) holds fields shared between draft and issued forms.
- **Issuance pattern**: `Issue*(draft, *series, signer, sourceID, now, opts, qr)` — validates draft, assigns next number in series, builds canonical string, signs into per-series **hash chain** (each signature covers previous document's hash), stamps ATCUD + QR code, returns immutable issued value. `IssuePayment` deliberately takes no `sourceID`/`signer` params — receipts carry no Hash/HashControl in SAF-T.
- **Signing contract**: `domain.Signer` interface; `Hash`/`HashControl` (`hash.go`) enforce XSD lengths + HashControl pattern, including recovery-form prefixes.
- **Recovery** (`recovery.go`, explained in `docs/recovery.md`): re-issue documents created outside certified system (manual `'M'` / backup `'D'`), provenance encoded in `HashControl` + `SourceBilling`.
- **Allocations** (`allocation.go`): receipt lines settling invoices / NC-ND lines rectifying them. Domain stays pure — caller fetches `SourceDocState` from persistence, `ValidateAllocations` applies rules; `AllocationPolicy` relaxes checks with no legal basis (unknown pre-system sources, rappel NCs over ceiling).
- Money is integer cents (`money.go`); dates/timestamps Europe/Lisbon.

### `internal/adapter/saft` — SAF-T (PT) projector

Projects domain values to XML, validates against `SAFTPT_1_04_01.xsd`, encoded Windows-1252. Entry point `Export(hdr Header, ...)` in `export.go`; per-section builders in `header.go`, `masterfiles.go`, `sales.go`, `working.go`, `movement.go`, `payments.go`. `saft.SoftwareIdentity` is caller-mapped producer metadata for `AuditFile/Header`.

### `internal/adapter/at` — AT webservice SOAP client

Client side of three AT webservices: **SeriesWS** (registar/finalizar/anular/consultarSeries, Portaria 195/2020), **sgdtws** (transport docs), **fatcorews** (real-time invoice communication, DL 28/2019). Ported from v1; wire-format quirks are load-bearing and documented inline.

- Ports in `at.go`: `SeriesClient` / `TransportClient` / `InvoiceClient`. Two implementations: `Client` (real SOAP, `client.go`) and `NullClient` (in-memory fake with deterministic SHA-256 validation codes, `null.go`).
- `RegistrationFor` / `FinalizationFor` / `CancellationFor` derive requests from `domain.Series` and encode AT rules (never-issued series → cancel not finalize; recovery series → tipoSerie `"R"`).
- `crypto.go`: WS-Security username token — password AES-128-ECB encrypted under nonce, nonce RSA-encrypted with AT cipher public key. ECB is AT's requirement, not a bug.
- `retry.go`: exponential backoff on `Error.IsRetryable()` codes only.
- Test vs production endpoints are `at.Test*URL` / `at.Production*URL` constants on `at.Config` — switch-over is config, not code. Production migration steps + gotchas in `docs/at-production.md` (everything live-verified against test env 2026-06-05).

### `internal/adapter/signing` — RSA-SHA1 signer (Portaria 363/2010 Art. 5)

`NewRSASigner(pemBytes, keyVersion)` satisfies `domain.Signer`.

### `internal/config`

`.env` loader (real env vars beat file) + `SoftwareIdentity` validation.

### `cmd/demo`

Runs 13 AT certification checklist scenarios (§5.1–5.13) end-to-end through domain layer.

### `cmd/atsmoke`

Exercises the AT SeriesWS **test environment** live: register throwaway series, consult, cancel (leaves env clean). `AT_TEST_COMM_ENABLED=1` adds fatcorews + sgdtws document communication; `AT_TEST_LOG_BODIES=1` dumps SOAP XML (passwords masked). Needs Portal das Finanças sub-user with WSE permission. Its `smoke_test.go` runs offline (no network).

## Invariants to respect

- Issued documents immutable; per-series hash chain must never break. Changing canonical-string or projector output for already-tested cases = regulatory bug, not refactor.
- Plan's persistence decision (M2+): stored SAF-T fragments are frozen record-of-truth, never regenerated.
- `ARCHITECTURE_PLAN.md` is roadmap — implement milestone-by-milestone, checkbox steps, stop at Checkpoints.