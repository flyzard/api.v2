# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Go library + application service layer. Issue Portuguese (AT-certified) tax documents, render them as PDF, export as SAF-T (PT) XML, communicate series/invoices/transport docs to AT webservices. Pure domain + adapters + a multi-tenant `app` service layer (Ports & Adapters), run by smoke binaries — a REST transport on top is the direction of travel. Design specs live in `docs/superpowers/specs/`, step-by-step implementation plans in `docs/superpowers/plans/`.

**Git policy: user owns all version control. Never run git commands (commit, branch, etc.) unless explicitly asked.** Where a plan says "Checkpoint", stop, let user commit.

## Commands

```bash
go build ./...                                  # build
go test ./...                                   # all tests
go test ./internal/domain -run TestName -v      # single test
go run ./cmd/appsmoke                           # §5 cert walkthrough via internal/app (memstore, stub signer); writes out-appsmoke/ SAF-T, PDFs, CHECKLIST.txt
go run ./cmd/atsmoke                            # live smoke vs AT *test* webservices; needs AT_NIF/AT_USERNAME/AT_PASSWORD + certs (see cmd/atsmoke/main.go doc comment)
```

!IMPORTANT: Follow YAGNI principles and one-liner solutions.

No Makefile, no linter config — `gofmt` only. `cmd/atsmoke` reads `.env` (real env vars beat file); `cmd/appsmoke` is self-contained (stub signer, hardcoded software identity — no signing key). `PRODUCER_TAX_ID`, `SOFTWARE_NAME`, `PRODUCER_NAME`, `VERSION`, `CERTIFICATE_NUMBER` feed `config.SoftwareIdentity`, validated at boot.

## Architecture

Strict dependency direction: `cmd/*` → `internal/app` → `internal/adapter/*` / `internal/config` → `internal/domain`. Domain imports nothing from adapters or app; adapters import only domain; `app` orchestrates domain + adapters behind ports.

### `internal/domain` — pure business logic

- **Document families**, each with own draft type + `Issue*` constructor: sales invoices (FT/FS/FR/NC/ND, `sales_invoice.go`), work documents (`work_document.go`), payments/receipts (RC/RG, `payment.go`), stock movements (`stock_movement.go`). `document_type.go` maps each `DocumentType` to family + rules. `DocumentCore` (`document.go`) holds fields shared between draft and issued forms.
- **Issuance pattern**: `Issue*(draft, *series, signer, sourceID, now, opts, qr)` — validates draft, assigns next number in series, builds canonical string, signs into per-series **hash chain** (each signature covers previous document's hash), stamps ATCUD + QR code, returns immutable issued value. `IssuePayment` deliberately takes no `sourceID`/`signer` params — receipts carry no Hash/HashControl in SAF-T.
- **Signing contract**: `domain.Signer` interface; `Hash`/`HashControl` (`hash.go`) enforce XSD lengths + HashControl pattern, including recovery-form prefixes.
- **Recovery** (`recovery.go`, explained in `docs/recovery.md`): re-issue documents created outside certified system (manual `'M'` / backup `'D'`), provenance encoded in `HashControl` + `SourceBilling`.
- **Allocations** (`allocation.go`): receipt lines settling invoices / NC-ND lines rectifying them. Domain stays pure — caller fetches `SourceDocState` from persistence, `ValidateAllocations` applies rules; `AllocationPolicy` relaxes checks with no legal basis (unknown pre-system sources, rappel NCs over ceiling).
- Money is integer cents (`money.go`); dates/timestamps Europe/Lisbon.

### `internal/app` — multi-tenant application service layer

Orchestrates domain + adapters behind persistence/infra **ports** (`ports.go`): `UnitOfWork.Run(ctx, tenantID, fn)` hands the callback a tenant-bound `RepoSet` (Series/Documents/Outbox/Idempotency repos); `OutboxQueue` is the worker-side cross-tenant port; `TenantStore`, `Clock`, `ATClientFactory` complete the wiring surface `Deps` (`app.go`). `New(Deps)` returns the five services.

- **`InvoicingService`** (`invoicing.go`): the issuance spine. Per `(tenant, series)` in-process mutex (`locks.go`) serializes the hash chain on the fast path; optimistic `Series.Version` check + up-to-3 retries is the cross-process backstop. Client-supplied idempotency key + payload fingerprint dedupes retries (same key, different payload → conflict). Document save, series advance, comm-task enqueue, and idempotency record all commit in one transaction. Issuance methods take a per-family `Issue*Request` struct (`IssueSalesInvoiceRequest`/`IssueWorkDocumentRequest`/`IssueStockMovementRequest`/`IssuePaymentRequest`) bundling `Draft`+`SeriesID`(+`SourceID`)+`Idem`; `Draft` stays a domain type — thin boundary, domain values are the wire contract, no DTO layer. `IssuePaymentRequest` is deliberately asymmetric: it carries `Totals` (`domain.IssuePayment` takes caller-supplied totals) and no `SourceID` (receipts carry no Hash/HashControl) — don't "normalise" it. `app.Status(Kind) int` and `app.Fingerprint([]byte) string` are the transport-seam helpers (HTTP status mapping; canonical idempotency fingerprint).
- **`CommService`** (`communication.go`): drains the AT-communication outbox. `DrainOnce` is the unit of work; the ticker loop belongs in the composition root. Exponential backoff, terminal failure after `maxCommAttempts`. Invoice communication is a per-tenant DL 28/2019 election (`Tenant.CommMode`).
- **`SeriesService`** (`series.go`): series lifecycle vs AT SeriesWS. AT-mutating operations never hold a transaction across the SOAP call.
- **`ExportService`** / **`QueryService`** (`export.go`, `query.go`): SAF-T export for a period; read side (fetch/list, comm status join, PDF rendering).
- **Errors** (`errors.go`): services return only `*app.Error{Kind, Err}`; `Kind` maps to HTTP status (`KindInvalid`/`NotFound`/`Conflict`/`AT`/`Internal`). Repos return sentinel `ErrNotFound`/`ErrVersionConflict`/`ErrAlreadyExists`; services translate.
- **`Tenant`** (`tenant.go`): one issuing company — `domain.Company`, AT sub-user credentials, `CommMode`, holiday calendar, FS limits. The signing key is NOT per-tenant; it's the global software-producer key (`Deps.Signer`, Portaria 363/2010).

### `internal/adapter/memstore` — in-memory ports implementation

Implements the `app` persistence ports for tests/demo. Tenant-first map keys (one tenant can never touch another's rows); `Run` holds one mutex for the whole transaction and snapshot/restores maps on error for all-or-nothing semantics. Has a `failSeriesSaveOnce` hook to exercise the service retry path.

### `internal/adapter/saft` — SAF-T (PT) projector

Projects domain values to XML, validates against `SAFTPT_1_04_01.xsd`, encoded Windows-1252. Entry point `Export(hdr Header, ...)` in `export.go`; per-section builders in `header.go`, `masterfiles.go`, `sales.go`, `working.go`, `movement.go`, `payments.go`. `saft.SoftwareIdentity` is caller-mapped producer metadata for `AuditFile/Header`.

### `internal/adapter/at` — AT webservice SOAP client

Client side of three AT webservices: **SeriesWS** (registar/finalizar/anular/consultarSeries, Portaria 195/2020), **sgdtws** (transport docs), **fatcorews** (real-time invoice communication, DL 28/2019). Ported from v1; wire-format quirks are load-bearing and documented inline.

- Ports in `at.go`: `SeriesClient` / `TransportClient` / `InvoiceClient`. Two implementations: `Client` (real SOAP, `client.go`) and `NullClient` (in-memory fake with deterministic SHA-256 validation codes, `null.go`).
- `RegistrationFor` / `FinalizationFor` / `CancellationFor` derive requests from `domain.Series` and encode AT rules (never-issued series → cancel not finalize; recovery series → tipoSerie `"R"`).
- `crypto.go`: WS-Security username token — password AES-128-ECB encrypted under nonce, nonce RSA-encrypted with AT cipher public key. ECB is AT's requirement, not a bug.
- `retry.go`: exponential backoff on `Error.IsRetryable()` codes only.
- Test vs production endpoints are `at.Test*URL` / `at.Production*URL` constants on `at.Config` — switch-over is config, not code. Production migration steps + gotchas in `docs/at-production.md` (everything live-verified against test env 2026-06-05).

### `internal/adapter/pdf` — PDF projector

Renders issued documents as A4 PDFs meeting AT print requirements (QR ≥30×30 mm, ATCUD above QR, signature characters + "Processado por programa certificado", per-family legal mentions). Pure projector like `saft` — consumes immutable domain values, never mutates. Golden-file tests in `testdata/`.

### `internal/adapter/qrimage` — QR rasterizer

Rasterizes frozen `IssuedDocument.QRPayload` strings to PNG. Enforces symbol version ≥ 9 and error-correction level exactly M **unconditionally** — a previous AT certification trial was rejected for a version below 9 (QR libraries auto-pick the smallest version that fits). Don't relax these invariants.

### `internal/adapter/signing` — RSA-SHA1 signer (Portaria 363/2010 Art. 5)

`NewRSASigner(pemBytes, keyVersion)` satisfies `domain.Signer`.

### `internal/config`

`.env` loader (real env vars beat file) + `SoftwareIdentity` validation.

### `cmd/appsmoke`

Runs the 13 AT certification checklist scenarios (§5.1–5.13) end-to-end through the `internal/app` service layer (memstore-backed, stub signer). Fixtures, scenarios, and the SAF-T (via `app.ExportService`) / PDF / checklist artefacts are local to the binary; writes `out-appsmoke/`.

### `cmd/atsmoke`

Exercises the AT SeriesWS **test environment** live: register throwaway series, consult, cancel (leaves env clean). `AT_TEST_COMM_ENABLED=1` adds fatcorews + sgdtws document communication; `AT_TEST_LOG_BODIES=1` dumps SOAP XML (passwords masked). Needs Portal das Finanças sub-user with WSE permission. Its `smoke_test.go` runs offline (no network).

## Invariants to respect

- Issued documents immutable; per-series hash chain must never break. Changing canonical-string or projector output for already-tested cases = regulatory bug, not refactor.
- Persistence decision: stored SAF-T fragments are frozen record-of-truth, never regenerated.
- QR images: version ≥ 9, ECC level M — AT rejected a prior trial over this.
- Work proceeds plan-by-plan from `docs/superpowers/plans/` — checkbox steps, stop at Checkpoints for user commits.
