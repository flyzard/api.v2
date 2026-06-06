# Invoice API — Architecture Implementation Plan

> **For agentic workers:** Implement milestone-by-milestone. Steps use checkbox (`- [ ]`) syntax. **Git policy: the user owns all version control.** Where a step says "Checkpoint", STOP and let the user commit — do not run git yourself.

**Goal:** Evolve the pure `domain` + `saft` library into a multi-tenant public REST API that issues Portuguese tax documents, persists them in Postgres with an unbroken per-series hash chain, and exports SAF-T asynchronously.

**Scope boundary:** v1 issues **SalesInvoices only** (FT/FS/FR/NC/ND). The domain/saft also model MovementOfGoods, WorkingDocuments, and Payments — those get their own series, per-document projector, and use cases in a later phase. Stated as an explicit boundary, not an omission.

**Architecture:** Ports & Adapters. `domain` stays pure. Use cases live in `app` and own transactions. Adapters (`http`, `postgres`, `saft`, `signing`, `blob`) plug into ports defined consumer-side in `app`.

**Tech Stack:** Go 1.26 · Huma v2 (`humago` stdlib adapter) · sqlc (`pgx/v5`) · goose · PostgreSQL (RLS-enforced multi-tenancy) · testcontainers-go.

**Decisions locked:**
- **`humago` (stdlib net/http) adapter** — fewest deps.
- **`pgx/v5`** — `FOR UPDATE` + `SKIP LOCKED`.
- **Persistence = frozen SAF-T XML record-of-truth + the exact signed canonical string + index/figure columns.** Each issued document stores: the `<Invoice>` fragment (frozen submission form), `signed_payload` (the exact string the signature covers), and indexed columns (number, hash, dates, status, net/gross totals). The XSD is the stable contract — domain structs refactor freely; closed periods re-export byte-stable. JSON for an API read-model is a **disposable derived cache**, never source of truth.
- **Referenced master data, split by drift exposure:**
  - **Customers — freely mutable.** Customer fields appear **only** in MasterFiles (the invoice carries `CustomerID` alone), so they cannot drift against frozen invoice lines. MasterFiles is built from current customer rows. No locking, no versioning.
  - **Products — immutable-after-use.** `ProductDescription` appears on frozen invoice lines **and** in MasterFiles, and AT requires one description per `ProductCode` across a file (cert §5.6/§5.10). So once a product is referenced by an issued document its SAF-T fields are frozen (`locked=true`); an edit is **rejected** — "changing" a product means creating a new one with a new `ProductCode`. This makes a mid-period change a loud *write-time* rejection instead of a silent *export-time* failure.
- **Multi-tenancy enforced by Postgres RLS**, per-table, `FORCE`d, fail-closed (`current_setting('app.company_id', true)`). **Two DB roles:** the tenant-scoped app role (non-owner, RLS-subject); a `BYPASSRLS` worker role used only by M6 to claim jobs cross-tenant, which then sets `app.company_id` per job before touching that tenant's documents.
- **sqlc reads goose migrations as schema** — single source of truth.
- **Ports final-signature from first use** — `Signer.Sign(canonical string)` (pure crypto, returns `error`), `Clock` port.

**Scope (Decision 2): stub now, harden later.** M2 proves the concurrency engine with a clearly-marked stub ATCUD (safe — nothing submits to AT yet). `status` column pulled forward; real series-registration/ATCUD and void/cancel are M3. **Ordering invariant: M5 (public API) must not ship before M3 (real ATCUD)**, or real invoices would carry `STUB-…` ATCUDs.

**Milestones (each ships green):**
- **M0** — Restructure into `/internal`.
- **M1** — Master data: companies, customers (mutable), products (immutable-after-use) + repos + RLS + two roles.
- **M2** — Issuance spine: series, documents, idempotency, hash chain, signed_payload, per-document projector, concurrency gate. *(highest risk)*
- **M3** — Series registration, real ATCUD, void/cancel.
- **M4** — Real RSA signer (Portaria 363/2010 Art. 5).
- **M5** — HTTP API: Huma operations (customers, products, issue, query), auth, idempotency, read-model.
- **M6** — Async SAF-T export: job queue + worker (BYPASSRLS role) + blob + fragment reassembly.

M3–M6 are outlined; each expands into its own plan when reached.

---

## M0 — Restructure into `/internal` (mechanical, no behavior change)

**Target:** `/cmd/demo`, `/internal/domain`, `/internal/adapter/saft`.

- [x] **Step 1: Move** — `mkdir -p internal/adapter cmd/demo` then `mv domain internal/domain` ; `mv saft internal/adapter/saft` ; `mv cmd/*.go cmd/demo/`. *(Also moved `signing → internal/adapter/signing` and `config → internal/config` — both post-date this plan.)*
- [x] **Step 2: Rewrite imports + format**
```bash
grep -rl 'invoicing.v2/domain' --include='*.go' . | xargs sed -i '' 's#invoicing.v2/domain#invoicing.v2/internal/domain#g'
grep -rl 'invoicing.v2/saft'   --include='*.go' . | xargs sed -i '' 's#invoicing.v2/saft#invoicing.v2/internal/adapter/saft#g'
gofmt -w .
```
- [x] **Step 3: Grep hardcoded paths** — `grep -rn 'saftpt1.04_01.xsd\|"saft/\|"domain/' --include='*.go' .` ; fix layout-dependent paths. *(Only hit: a comment in `export_test.go` — no fix needed.)*
- [x] **Step 4: Review diff** — `git diff`; only import paths should change. *(Verified per-file vs HEAD: import lines only.)*
- [x] **Step 5: Build + test** — `go build ./... && go test ./...` → green.
- [x] **Step 6: Byte-identical** — `go run ./cmd/demo > /dev/null && wc -c out/SAFT-DEMO-2026-05.xml` → `59513`. *(Plan's `58437` predates recovery work; baseline re-measured pre-move same session.)*
- [x] **Checkpoint (you commit):** "refactor: move to internal/ layout"

---

## M1 — Master data (companies, customers, products)

**Files:** `sql/migrations/001…003`, `sql/queries/master.sql`, `sqlc.yaml`, `internal/adapter/postgres/{pool.go,master_repo.go}`, `internal/app/{ports.go,master.go}`.

- [ ] **Step 1: Migrations (RLS folded per-table)**

`001_companies.sql`:
```sql
-- +goose Up
CREATE TABLE companies (
    id UUID PRIMARY KEY, nif TEXT NOT NULL, name TEXT NOT NULL,
    eac_code TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE companies ENABLE ROW LEVEL SECURITY;
ALTER TABLE companies FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant ON companies USING (id = current_setting('app.company_id', true)::uuid);
-- +goose Down
DROP TABLE companies;
```

`002_customers.sql` — **freely mutable** (no `locked`):
```sql
-- +goose Up
CREATE TABLE customers (
    id UUID PRIMARY KEY,
    company_id UUID NOT NULL REFERENCES companies(id),
    account_id TEXT NOT NULL,
    customer_tax_id TEXT NOT NULL,
    company_name TEXT NOT NULL,
    contact TEXT, building_number TEXT, street_name TEXT,
    address_detail TEXT NOT NULL, city TEXT NOT NULL, postal_code TEXT NOT NULL,
    region TEXT, country TEXT NOT NULL,
    telephone TEXT, fax TEXT, email TEXT, website TEXT,
    self_billing BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, account_id)
);
ALTER TABLE customers ENABLE ROW LEVEL SECURITY;
ALTER TABLE customers FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant ON customers USING (company_id = current_setting('app.company_id', true)::uuid);
-- +goose Down
DROP TABLE customers;
```

`003_products.sql` — **immutable-after-use** (`locked`; no delete once locked):
```sql
-- +goose Up
CREATE TABLE products (
    id UUID PRIMARY KEY,
    company_id UUID NOT NULL REFERENCES companies(id),
    product_code TEXT NOT NULL,
    product_type TEXT NOT NULL,
    product_group TEXT,
    product_description TEXT NOT NULL,
    product_number_code TEXT NOT NULL,
    unit TEXT NOT NULL,
    locked BOOLEAN NOT NULL DEFAULT false,   -- true once referenced by an issued doc
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, product_code)
);
ALTER TABLE products ENABLE ROW LEVEL SECURITY;
ALTER TABLE products FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant ON products USING (company_id = current_setting('app.company_id', true)::uuid);
-- +goose Down
DROP TABLE products;
```
> `current_setting('app.company_id', true)` (missing_ok) → unset yields NULL → zero rows (fail-closed). App connects as a **non-owner role**.

- [ ] **Step 2: sqlc config** — `sqlc.yaml`
```yaml
version: "2"
sql:
  - engine: postgresql
    schema: "sql/migrations"
    queries: "sql/queries"
    gen:
      go: { package: "db", out: "internal/adapter/postgres/db", sql_package: "pgx/v5", emit_pointers_for_null_types: true }
```

- [ ] **Step 3: master queries** — `sql/queries/master.sql`:
`CreateCustomer`, `UpdateCustomer`, `GetCustomer`, `ListCustomersByIDs`;
`CreateProduct`, `GetProduct`, `LockProductForUse` (`SELECT … FOR UPDATE`), `MarkProductLocked`, `ListProductsByIDs`.
Run `sqlc generate`.

- [ ] **Step 4: tenant plumbing + roles** — `internal/adapter/postgres/pool.go` with `WithTenant(ctx, companyID, fn)` (txn + `SELECT set_config('app.company_id', $1, true)`, hands fn a scoped `db.Queries`). **Every** read/write goes through it. Document the two roles (app non-owner; worker `BYPASSRLS` for M6).

- [ ] **Step 5: ports + use cases** — `internal/app/ports.go` (`CustomerRepo`, `ProductRepo`) and `internal/app/master.go`:
  - `RegisterCustomer` / `UpdateCustomer` — build `domain.Customer` via ctor, persist. **Edits always allowed.**
  - `RegisterProduct` — build `domain.Product`, persist (`locked=false`).
  - `UpdateProduct` — **reject if `locked`** with a typed error (`ErrProductLocked`); caller must create a new `product_code`.

- [ ] **Step 6: tests** — RLS isolation (A can't read B even by id); customer update succeeds; product update on a `locked` row → `ErrProductLocked`; round-trip (row → `domain.Customer`/`domain.Product` → projector `buildCustomer`/`buildProduct` matches input).
Run: `go test ./internal/adapter/postgres ./internal/app -run 'Master|RLS|Locked' -v` → PASS.

- [ ] **Checkpoint (you commit):** "feat: tenant-scoped master data (mutable customers, immutable-after-use products) + RLS"

---

## M2 — Issuance spine + hash chain (highest risk)

**Files:** `sql/migrations/004_document_series.sql`, `005_documents.sql`, `006_idempotency.sql`; `sql/queries/documents.sql`; `internal/adapter/saft/document.go`; `internal/adapter/signing/stub.go`; `internal/app/{ports.go (extend),issue.go,verify.go}`; tests.

- [ ] **Step 1 (RISK GATE): per-document projector** — `internal/adapter/saft/document.go`
```go
// ExportInvoice projects one sales invoice to its <Invoice> SAF-T fragment
// (UTF-8). M6 reassembles fragments verbatim into the SalesInvoices container
// and transcodes the whole file to Windows-1252. INVARIANT: a stored fragment
// is never regenerated.
func ExportInvoice(d domain.SalesInvoice, issuerEAC string) ([]byte, error) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	start := xml.StartElement{Name: xml.Name{Local: "Invoice"}}
	if err := enc.EncodeElement(buildInvoice(d, issuerEAC), start); err != nil {
		return nil, fmt.Errorf("project invoice: %w", err)
	}
	if err := enc.Flush(); err != nil { return nil, err }
	return buf.Bytes(), nil
}
```
Golden test asserts `<Invoice>`, `<InvoiceNo>FT FT2026/1</InvoiceNo>`, `<Hash>`, `<EACCode>…`, `</Invoice>`. Run → PASS.

- [ ] **Step 2: migrations**

`004_document_series.sql` — **the spine**:
```sql
-- +goose Up
CREATE TABLE document_series (
    company_id UUID NOT NULL REFERENCES companies(id),
    doc_type TEXT NOT NULL, series TEXT NOT NULL,
    last_number BIGINT NOT NULL DEFAULT 0,
    last_hash TEXT NOT NULL DEFAULT '',                       -- '' feeds the first signature (Portaria)
    last_system_entry TIMESTAMPTZ NOT NULL DEFAULT 'epoch',   -- SystemEntryDate monotonicity
    -- validation_code (AT-assigned) added in M3
    PRIMARY KEY (company_id, doc_type, series)
);
ALTER TABLE document_series ENABLE ROW LEVEL SECURITY;
ALTER TABLE document_series FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant ON document_series USING (company_id = current_setting('app.company_id', true)::uuid);
-- +goose Down
DROP TABLE document_series;
```

`005_documents.sql`:
```sql
-- +goose Up
CREATE TABLE documents (
    id UUID PRIMARY KEY,
    company_id UUID NOT NULL REFERENCES companies(id),
    customer_id UUID NOT NULL REFERENCES customers(id),
    doc_type TEXT NOT NULL, series TEXT NOT NULL, number BIGINT NOT NULL,
    formatted_no TEXT NOT NULL, atcud TEXT NOT NULL,          -- STUB in M2; real in M3
    hash TEXT NOT NULL, hash_control TEXT NOT NULL,
    signed_payload TEXT NOT NULL,                              -- exact canonical string the signature covers
    issue_date DATE NOT NULL, system_entry TIMESTAMPTZ NOT NULL, period INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'N',                          -- 'N' normal, 'A' cancelled (void = M3)
    net_total BIGINT NOT NULL, gross_total BIGINT NOT NULL,    -- cents; aggregate recompute w/o XML parse
    saft_xml TEXT NOT NULL,                                    -- frozen <Invoice> fragment
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, doc_type, series, number)
);
CREATE INDEX idx_documents_period ON documents (company_id, issue_date);
ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE documents FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant ON documents USING (company_id = current_setting('app.company_id', true)::uuid);
-- +goose Down
DROP TABLE documents;
```

`006_idempotency.sql`: `PRIMARY KEY (company_id, key)`, FK `document_id`, RLS policy (as the others).

- [ ] **Step 3: queries** — `sql/queries/documents.sql`: `LockSeries` (`FOR UPDATE`), `AdvanceSeries` (last_number/last_hash/last_system_entry), `InsertDocument`, `GetIdempotent`, `PutIdempotent`, `GetResult`, `ListChain`, `ListXMLForPeriod`. `sqlc generate`.

- [ ] **Step 4: extend ports** — `internal/app/ports.go`
```go
type Series struct {
	CompanyID uuid.UUID; DocType, Series string
	LastNumber int64; LastHash string; LastSystemEntry time.Time
}
type Result struct{ ID uuid.UUID; FormattedNo, Hash string }

type DocRepo interface {
	WithTenant(ctx context.Context, companyID uuid.UUID, fn func(DocRepo) error) error
	LockSeries(ctx context.Context, companyID uuid.UUID, docType, series string) (Series, error)
	AdvanceSeries(ctx context.Context, s Series) error
	// LockProductsForUse locks the given product rows FOR UPDATE in id order
	// (deadlock-safe) and marks any unlocked ones locked=true; returns the
	// domain.Products for assembling the draft.
	LockProductsForUse(ctx context.Context, companyID uuid.UUID, productIDs []uuid.UUID) ([]domain.Product, error)
	GetCustomer(ctx context.Context, companyID, id uuid.UUID) (domain.Customer, error)
	InsertDocument(ctx context.Context, d StoredDoc) error
	GetResult(ctx context.Context, companyID, id uuid.UUID) (Result, error)
	Idempotent(ctx context.Context, companyID uuid.UUID, key string) (uuid.UUID, bool, error)
	SaveIdempotent(ctx context.Context, companyID uuid.UUID, key string, docID uuid.UUID) error
}
type Signer interface{ Sign(canonical string) (hash, control string, err error) } // pure crypto
type Clock  interface{ Now() time.Time }
```

- [ ] **Step 5: IssueDocument use case** — `internal/app/issue.go`

Order: **lock series → idempotency (post-lock) → load customer (no lock) + lock+read products → stamp (monotonic) → canonical → sign → freeze → persist → advance → record idempotency**, one tenant txn.
```go
func (s *Service) IssueDocument(ctx context.Context, cmd IssueCmd) (Result, error) {
	var out Result
	err := s.repo.WithTenant(ctx, cmd.CompanyID, func(r DocRepo) error {
		st, err := r.LockSeries(ctx, cmd.CompanyID, cmd.DocType, cmd.Series)        // 1. serialize chain
		if err != nil { return err }
		if id, ok, err := r.Idempotent(ctx, cmd.CompanyID, cmd.IdempotencyKey); err != nil {
			return err
		} else if ok { out, err = r.GetResult(ctx, cmd.CompanyID, id); return err } // 2. replay
		cust, err := r.GetCustomer(ctx, cmd.CompanyID, cmd.CustomerID)              // 3a. customer (mutable, no lock)
		if err != nil { return err }
		prods, err := r.LockProductsForUse(ctx, cmd.CompanyID, cmd.ProductIDs)      // 3b. lock+freeze products
		if err != nil { return err }
		doc, err := assembleDraft(cmd, cust, prods)                                 //     build domain doc; domain computes totals
		if err != nil { return err }
		doc.SystemEntryDate = maxTime(s.clock.Now(), st.LastSystemEntry)            // 4. monotonic, Europe/Lisbon
		doc.Number, err = domain.NewDocNumber(domain.DocType(cmd.DocType), cmd.Series, st.LastNumber+1)
		if err != nil { return err }
		doc.ATCUD = domain.ATCUD(stubATCUD(cmd.Series, st.LastNumber+1))            //    STUB → M3
		canonical := doc.Canonical(st.LastHash)                                     // 5. domain owns format
		h, c, err := s.signer.Sign(canonical)                                       // 6. pure-crypto signer
		if err != nil { return err }
		doc.Hash, doc.HashControl = domain.Hash(h), domain.HashControl(c)
		xmlFrag, err := saft.ExportInvoice(doc, cmd.IssuerEAC)                       // 7. freeze
		if err != nil { return err }
		docID := uuid.New()
		if err := r.InsertDocument(ctx, toStored(cmd.CompanyID, docID, doc, canonical, xmlFrag)); err != nil {
			return err                                                              // 8. persist (incl signed_payload)
		}
		st.LastNumber, st.LastHash, st.LastSystemEntry = st.LastNumber+1, h, doc.SystemEntryDate
		if err := r.AdvanceSeries(ctx, st); err != nil { return err }               // 9. advance chain
		if err := r.SaveIdempotent(ctx, cmd.CompanyID, cmd.IdempotencyKey, docID); err != nil { return err }
		out = Result{ID: docID, FormattedNo: doc.Number.Format(), Hash: h}
		return nil
	})
	return out, err
}
```
`stubATCUD` = `fmt.Sprintf("STUB-%s-%d", series, n)`. `assembleDraft` builds lines from the locked products + client qty/price/tax; domain computes totals. `Canonical(prevHash)` + `maxTime` are M2 additions. Stamp/store/render SystemEntryDate consistently in **Europe/Lisbon**.
> Confirm the domain ctor validates `Period == month(issue Date)`.

- [ ] **Step 6: repo + stub signer** — `internal/adapter/postgres/doc_repo.go` (`WithTenant` = txn + `set_config`; `LockProductsForUse` sorts IDs, `FOR UPDATE`, sets `locked`), `internal/adapter/signing/stub.go`.

- [ ] **Step 7: VerifyChain** — `internal/app/verify.go`: `ListChain` in number order; assert contiguous numbers + chain linkage from `signed_payload`. (Full crypto re-verification in M4.)

- [ ] **Step 8 (de-risking test): concurrency** — seed company+customer+product+series; fire **N=50** concurrent `IssueDocument` at one series → numbers exactly `1..N`, `VerifyChain` passes. Run → PASS.

- [ ] **Step 9: idempotency + monotonicity + product-lock tests** — same key twice concurrently → one row, same `FormattedNo`; `system_entry` non-decreasing with number; a product referenced by an issue becomes `locked` and subsequent `UpdateProduct` → `ErrProductLocked`. Run → PASS.

- [ ] **Checkpoint (you commit):** "feat: hash-chain issuance + frozen record-of-truth + concurrency gate"

---

## M3 — Series registration, real ATCUD, void/cancel (own plan)

(1) `validation_code` on `document_series` + `RegisterSeries` (manual now, AT webservice later). (2) `stubATCUD` → `{validation_code}-{number}`. (3) `VoidDocument`: under the series lock set `status='A'`, re-project the frozen fragment with cancelled status, keep the number in the chain. (4) **Resolve open rule:** do status-`A` docs contribute to family `TotalDebit/TotalCredit`? Confirm against AT before M6 aggregates. Tests: ATCUD format; voided doc still in period export with status `A`.

## M4 — Real RSA signer (own plan)

`Signer.Sign(canonical)` with Portaria 363/2010 Art. 5 (RSA over the canonical string, base64; `HashControl` = key version). `domain.Canonical(prevHash)` (M2) owns field order/format. Upgrade `VerifyChain` to full crypto re-verification from `signed_payload`. Acceptance: known-answer vector matches byte-for-byte; `VerifyChain` passes on M2's output.

## M5 — HTTP API: Huma (own plan) — must follow M3

`internal/adapter/http` + `cmd/api`. Ops: `POST/GET/PATCH /v1/customers` (PATCH allowed), `POST/GET /v1/products` (no PATCH on locked → 409), `POST /v1/documents` (+ required `Idempotency-Key` header), `GET /v1/documents/{id}`. Auth middleware: API key → `company_id` in context (never client-supplied). DTO↔command via domain ctors (ctor error → `huma.Error422`); conflict → 409. **Read-model:** GET/replay return the full document — parse stored `saft_xml`, or add a disposable `read_json` column at issue (rebuildable). `Result` is too thin for replay bodies → return the read-model. Tests: happy path, validation-422, idempotent-replay returns identical body, cross-tenant GET → 404, edit-locked-product → 409.

## M6 — Async SAF-T export (own plan)

`internal/adapter/blob`, `internal/app/export.go`, `cmd/worker`, `007_export_jobs.sql`. (1) `JobQueue` port + Postgres impl using `FOR UPDATE SKIP LOCKED`; the **worker connects as the `BYPASSRLS` role** to claim across tenants, then `set_config('app.company_id', job.company_id, true)` before reading that tenant's documents. (2) `BlobStore` port + S3 adapter. (3) Export use case: `ListXMLForPeriod` → **reassemble fragments verbatim** via a container field tagged `xml:",innerxml"` (fragments inherit the AuditFile default namespace — no string-concat), recompute family aggregates from `net_total`/`gross_total` columns, build MasterFiles from **current** `customers` + `products` rows referenced in the period (customers can't drift; products are locked so current == frozen), wrap in `AuditFile`+`Header`, transcode to Win-1252 → blob. (4) HTTP: `POST /v1/exports` → 202, `GET` status, signed-URL download. Acceptance: reassembled file validates against the XSD; two workers process each job exactly once.

---

## Self-Review

- **Master data (corrected):** customers mutable (MasterFiles-only fields → no drift); products immutable-after-use (`locked`, `ProductDescription` on frozen lines must match the single MasterFiles entry). Mid-period product change = write-time 409, not export-time failure. No snapshot/versioning tables.
- **Decision 1:** frozen `saft_xml` + `signed_payload` + figure columns. Per-doc projector M2 Step 1. Invariant: fragments never regenerated.
- **Concurrency:** series `FOR UPDATE` serializes the chain; `LockProductsForUse` (id-sorted, deadlock-safe) closes the first-use write-skew on products; customers need no lock.
- **RLS:** per-table, `FORCE`d, fail-closed; every read/write via `WithTenant`; two roles (app non-owner + worker `BYPASSRLS`).
- **Other fixes:** SystemEntryDate monotonic + Europe/Lisbon consistency (M2); `Signer.Sign(canonical)` decoupled; M6 reassembly pinned to `,innerxml`; read-model for M5; SalesInvoices-only scope boundary stated; M5-after-M3 ordering invariant; locked products not editable/deletable.
- **Open rules flagged:** voided-in-totals (→ M3, AT confirm); Period↔Date (→ M2 ctor). First-doc empty `prevHash` verified covered.
- **Type consistency:** `Series`, `Result`, `StoredDoc`, `DocRepo`, `CustomerRepo`, `ProductRepo`, `Signer`, `Clock`, `IssueCmd`, `saft.ExportInvoice`, `domain.Canonical` defined once, reused. Glue casts to existing domain types flagged, not invented.
- **No speculative code:** M0–M2 carry complete migrations, queries, ports, use case, projector, and three de-risking tests. M3–M6 are outlines.
