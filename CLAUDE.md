# CLAUDE.md

Guide Claude Code (claude.ai/code) for work in this repo.

## What this is

Go library. Issue Portuguese (AT-certified) tax documents, export as SAF-T (PT) XML. Now pure domain + projector library, run by demo binary. Evolving milestone-by-milestone into multi-tenant REST API per `ARCHITECTURE_PLAN.md` (Ports & Adapters; M0 restructure done, M1+ pending).

**Git policy: user owns all version control. Never run git commands (commit, branch, etc.) unless explicitly asked.** Where plan says "Checkpoint", stop, let user commit.

## Commands

```bash
go build ./...                                  # build
go test ./...                                   # all tests
go test ./internal/domain -run TestName -v      # single test
go run ./cmd/demo                               # AT certification §5.1–5.13 walkthrough; writes out/SAFT-DEMO-2026-05.xml
```

No Makefile, no linter config — `gofmt` only. Demo reads `.env` (optional); set `AT_SIGNING_KEY_FILE` for real RSA signatures, else stub signer used. `PRODUCER_TAX_ID`, `SOFTWARE_NAME`, `PRODUCER_NAME`, `VERSION`, `CERTIFICATE_NUMBER` feed `config.SoftwareIdentity`, validated at boot.

## Architecture

Three layers, strict dependency direction: `config` / `adapter/*` → `domain`. Domain imports nothing from adapters.

### `internal/domain` — pure business logic

- **Document families**, each with own draft type + `Issue*` constructor: sales invoices (FT/FS/FR/NC/ND, `sales_invoice.go`), work documents (`work_document.go`), payments/receipts (RC/RG, `payment.go`), stock movements (`stock_movement.go`). `document_type.go` maps each `DocumentType` to family + rules. `DocumentCore` (`document.go`) holds fields shared between draft and issued forms.
- **Issuance pattern**: `Issue*(draft, *series, signer, sourceID, now, opts, qr)` — validates draft, assigns next number in series, builds canonical string, signs into per-series **hash chain** (each signature covers previous document's hash), stamps ATCUD + QR code, returns immutable issued value. `IssuePayment` deliberately takes no `sourceID`/`signer` params — receipts carry no Hash/HashControl in SAF-T.
- **Signing contract**: `domain.Signer` interface; `Hash`/`HashControl` (`hash.go`) enforce XSD lengths + HashControl pattern, including recovery-form prefixes.
- **Recovery** (`recovery.go`, explained in `docs/recovery.md`): re-issue documents created outside certified system (manual `'M'` / backup `'D'`), provenance encoded in `HashControl` + `SourceBilling`.
- Money is integer cents (`money.go`); dates/timestamps Europe/Lisbon.

### `internal/adapter/saft` — SAF-T (PT) projector

Projects domain values to XML, validates against `SAFTPT_1_04_01.xsd`, encoded Windows-1252. Entry point `Export(hdr Header, ...)` in `export.go`; per-section builders in `header.go`, `masterfiles.go`, `sales.go`, `working.go`, `movement.go`, `payments.go`. `saft.SoftwareIdentity` is caller-mapped producer metadata for `AuditFile/Header`.

### `internal/adapter/signing` — RSA-SHA1 signer (Portaria 363/2010 Art. 5)

`NewRSASigner(pemBytes, keyVersion)` satisfies `domain.Signer`.

### `internal/config`

`.env` loader (real env vars beat file) + `SoftwareIdentity` validation.

### `cmd/demo`

Runs 13 AT certification checklist scenarios (§5.1–5.13) end-to-end through domain layer.

## Invariants to respect

- Issued documents immutable; per-series hash chain must never break. Changing canonical-string or projector output for already-tested cases = regulatory bug, not refactor.
- Plan's persistence decision (M2+): stored SAF-T fragments are frozen record-of-truth, never regenerated.
- `ARCHITECTURE_PLAN.md` is roadmap — implement milestone-by-milestone, checkbox steps, stop at Checkpoints.