# SAF-T (PT) Export — Plan of Execution

Target: a working Tier-3 SAF-T XML projector that consumes the issued documents
produced by `cmd/main.go` and emits a Windows-1252 XML file validating against
SAF-T PT 1.04_01 for the 13 AT-certification scenarios.

This is the *minimum viable export* for the certification walkthrough — not a
production-grade exporter. Scope decisions favour the cert checklist over
generality.

---

## 1. Goal

After `scenario513` finishes, `cmd/main.go` writes
`out/SAFT-DEMO-2026-05.xml` (Win-1252) containing:

- `Header` derived from the issuer `Company`, the `SoftwareIdentity`, and the
  export window (fiscal year + StartDate/EndDate).
- `MasterFiles/Customer` deduped over issued docs by `CustomerID`.
- `MasterFiles/Product` deduped by `ProductCode`.
- `MasterFiles/TaxTable` derived from `domain.taxRates`.
- `SourceDocuments/SalesInvoices` for every issued FT/FS/FR/NC/ND (incl. the
  cancelled FT from 5.2 with DocumentStatus = "A").
- `SourceDocuments/MovementOfGoods` for every issued GR/GT/GA/GC/GD (incl. the
  non-valued GR from 5.11b).
- `SourceDocuments/WorkingDocuments` for every issued NE/OR/PF/CM/FC/FO
  (incl. the NE from 5.3 transitioned to Status "F" by 5.4).
- `SourceDocuments/Payments` for every issued RC/RG.

Output XSD-validates under `SAFTPT_1_04_01.xsd` (validated externally via
`xmllint --schema`).

## 2. Non-goals (v1)

- Real RSA-SHA1 signing — `stubSigner` produces shaped 172-char hashes that
  round-trip through the projector. Note: stub uses `prev + "|" + canonical`,
  not AT's `<prev>;<canonical>` canonical format — AT verifier would reject,
  but v1 only targets XSD validity (see L1).
- AT product-code resolution for FS retail tier (`NOR_001` etc.) — emit
  declared product codes verbatim.
- Streaming/large-file mode — entire export is built in memory.
- In-process XSD validation — rely on `xmllint`.
- `.zip` packaging — emit `.xml` only.
- General-ledger / supplier blocks — `TaxAccountingBasis = "F"` opts us out.
- A second, "production" projector signature with persistence ports — the demo
  projector takes already-collected slices.

---

## 3. Pre-flight facts (verified)

| Question | Answer | Source |
|---|---|---|
| Is `domain/win1252.go` UTF-8→Win-1252 encoder available? | Yes via `charmap.Windows1252.NewEncoder()` — already a transitive dep. | `domain/win1252.go:6` |
| Where is the AT validation code stored? | `Series.ATCode`; redundant for export since `ATCUD` is on every issued doc. | `domain/series.go:29`, `domain/issued_document.go:134` |
| XSD revision? | SAF-T PT 1.04_01 (Portaria 195/2020 + 2025 M44/M45/M46 codes — already modelled). | `domain/tax.go:171-173` |
| Are docs already Lisbon-normalized? | Yes — `issueCommon` and `IssuePayment` call `.In(lisbonLocation)`. | `domain/issued_document.go:204` |
| Is `Totals.Breakdown` deterministic? | Yes — sorted by `(Region, Category, Exemption)`. | `domain/document.go:158` |
| Does `Breakdown` cover `NotSubjectTax` lines? | **No** — explicit TODO. None of the 13 scenarios trigger this, so non-blocking for v1. | `domain/document.go:103` |

---

## 4. Gaps in the domain that v1 must close

These are real shortfalls in what the projector needs vs. what is persisted.

### G1 — `memoryStore` discards every family-specific field
`memoryStore.docs` is `map[string]IssuedDocument`. The existing
`c.store.record(doc.IssuedDocument)` calls in `cmd/scenarios.go:65,74,83`
slice each typed value (`SalesInvoice`, `StockMovement`, `WorkDocument`) down
to its embedded `IssuedDocument` — losing `Settlement`, `Currency`, `ShipTo`,
`ShipFrom`, `MovementStartTime`/`EndTime`, `FRPayments`, `WithholdingTax`,
`ATDocCodeID`, `ThirdParties`, `SpecialRegimes`. `Payment` isn't stored at
all (RC/RG from §5.13 are issued, printed, dropped).

**Fix:** four typed maps, recorded by each family's issue helper.
```go
type memoryStore struct {
    sales    map[string]domain.SalesInvoice
    stock    map[string]domain.StockMovement
    work     map[string]domain.WorkDocument
    payments map[string]domain.Payment
}
```
`IssuedDocumentReader.FindByNumber` keeps working by reading from the sales
map and returning the embedded `IssuedDocument` (ND validation cares about
sales only). Estimate: ~80 LOC across `cmd/helpers.go` and `cmd/scenarios.go`.

### G2 — `SalesInvoice` has no place for doc-level `Settlement`
Scenario 5.7 emits a global discount via `printSettlementSimulation` only — it
is not persisted on the issued doc. SAF-T `Invoice/DocumentTotals/Settlement`
is required when present.

**Fix:** add `Settlement *Money` (and an optional description) to
`SalesInvoiceFields`. Populate it in 5.7. The projector emits the block iff
non-nil.

### G3 — `SalesInvoice` has no `Currency`
Scenario 5.8 emits a foreign-currency invoice via `printCurrencySimulation`.
SAF-T `Invoice/DocumentTotals/Currency` is required for non-EUR originals.

**Fix:** add `Currency *Currency` to `SalesInvoiceFields`. Populate it in 5.8.

### G4 — No `EACCode` plumbed into the issued doc (per-doc)
SAF-T `SalesInvoice/EACCode` is per-invoice but typically inherits from
issuer. Currently only `IssueOptions.IssuerEAC` exists (passed in for FS
threshold resolution) and is not stored.

**Fix v1:** the projector pulls EACCode from the `Header.Company` once and
emits the same value on every `SalesInvoice` / `WorkDocument` /
`StockMovement`. Domain change deferred.

### G6 — `Cancel()` does not capture the canceller's `SourceID`
SAF-T `DocumentStatus/SourceID` is the user who *applied the status*, not the
original issuer. `IssuedDocument.Cancel` writes `StatusDate` only — the
original `SourceID` stays. Single-user demo masks this.

**Fix v1:** projector emits the doc's original `SourceID` in
`DocumentStatus/SourceID` and documents the simplification. Promote to a
`Cancel(reason, at, cancelSourceID, clock)` signature change only when a
multi-user flow demands it.

### G5 — Header constants not in fixtures
Missing in `fixtures.go`: `TaxAccountingBasis`, `TaxEntity`,
`AuditFileVersion`, `CurrencyCode` (header), `DateCreated`, `StartDate`,
`EndDate`. All are constants or computable at export time — the projector owns
them, not the domain.

**Fix:** projector defines/exposes them; `main` passes `StartDate`/`EndDate`
when calling `Export`.

---

## 5. Package layout

```
saft/
  doc.go          — package overview, mapping table to AT XSD elements
  header.go       — Header struct + builder from Company + SoftwareIdentity
  masterfiles.go  — Customer/Product/TaxTable derivation (dedupe)
  sales.go        — SalesInvoices block + Invoice element + family totals
  movement.go     — MovementOfGoods + StockMovement element + qty/line totals
  working.go      — WorkingDocuments + WorkDocument element + family totals
  payments.go     — Payments + Payment element + family totals
  lines.go        — shared DocumentLine → SAF-T Line marshal (Tax dispatch)
  encode.go       — UTF-8→Win-1252 transcode + XML PI rewrite
  export.go       — Export(...) entry point assembling the AuditFile
  testdata/
    scenarios_golden.xml  — golden file produced by `cmd/main.go`
```

All under `github.com/flyzard/invoicing.v2/saft`.

Domain stays untouched except for G1/G2/G3 listed above. The projector reads
from `domain` types; it does not import or extend `cmd`.

## 6. Public surface

```go
// Header carries the AT-mandated header fields. Builder helpers handle the
// constants; callers supply only what varies per export.
type Header struct {
    AuditFileVersion        string    // const "1.04_01"
    CompanyID               string    // NIF or registry-code combo
    TaxRegistrationNumber   string    // NIF
    TaxAccountingBasis      string    // const "F"
    CompanyName             string
    BusinessName            string    // optional, == TradeName
    CompanyAddress          domain.Address
    FiscalYear              int
    StartDate, EndDate      time.Time // Europe/Lisbon
    CurrencyCode            string    // const "EUR"
    DateCreated             time.Time
    TaxEntity               string    // const "Global"
    ProductCompanyTaxID     string    // SoftwareIdentity.ProducerTaxID
    SoftwareCertificateNumber string  // SoftwareIdentity.CertificateNumber
    ProductID               string    // SoftwareIdentity.ProductID()
    ProductVersion          string    // SoftwareIdentity.Version
    HeaderComment           string    // optional
    Telephone, Fax, Email, Website string
}

func NewHeader(issuer domain.Company, sw domain.SoftwareIdentity,
               start, end time.Time, now time.Time) Header

// Export produces a Win-1252 XML byte slice ready to write to disk.
func Export(hdr Header,
            sales []domain.SalesInvoice,
            stock []domain.StockMovement,
            work  []domain.WorkDocument,
            payments []domain.Payment) ([]byte, error)
```

`Export` filters every input slice by `Date in [StartDate, EndDate]` (or
`TransactionDate` for payments).

## 7. SAF-T element mapping (subset emitted)

### 7.1 Header
- Constants: `AuditFileVersion=1.04_01`, `TaxAccountingBasis=F`,
  `TaxEntity=Global`, `CurrencyCode=EUR`.
- `CompanyAddress`: map `domain.Address` → `AddressDetail`, `City`,
  `PostalCode`, `Region`, `Country`.

### 7.2 MasterFiles
- `Customer`: dedupe by `CustomerID`. Anonymous customer (5.x retail FS) maps
  to `CustomerID=999999990`, `AccountID=ConsumidorFinal`, no
  `BillingAddress` (XSD allows it but most validators want a stub — emit
  `Desconhecido` country + zeroed fields if needed; verify against xmllint).
- `Product`: dedupe by `ProductCode`. Emit `ProductType`, `ProductCode`,
  `ProductGroup` (if set), `ProductDescription`, `ProductNumberCode`.
- `TaxTable`: walk `domain.taxRates`; emit one `TaxTableEntry` per
  `(Region, Category)` plus one per `Exemption` used in the document set
  (TaxType="IVA", TaxCountryRegion=Region, TaxCode=Category, TaxPercentage).

### 7.3 SourceDocuments — common per-doc fields
For each `IssuedDocument`-rooted family:

```
<InvoiceNo|DocumentNumber|PaymentRefNo>  ← Number.Format()
<ATCUD>                                   ← ATCUD
<DocumentStatus>
  <InvoiceStatus|MovementStatus|WorkStatus|PaymentStatus> ← Status
  <InvoiceStatusDate ...>                 ← StatusDate (2006-01-02T15:04:05)
  <Reason? />                             ← Reason (when Status="A")
  <SourceID>                              ← SourceID
  <SourceBilling|SourcePayment>           ← SourceBilling
</DocumentStatus>
<Hash>                                    ← Hash
<HashControl>                             ← HashControl
<Period>                                  ← Period
<InvoiceDate|MovementDate|WorkDate>       ← Date (2006-01-02)
<InvoiceType|MovementType|WorkType>       ← DocumentType
<SpecialRegimes>                          ← SpecialRegimes (sales only)
<SourceID/>                               ← IssuedBy.Email
<EACCode/>                                ← Header.Company.EACCode
<SystemEntryDate>                         ← SystemEntryDate
<CustomerID>                              ← Customer.CustomerID.String()
<Line>… (see 7.4)
<DocumentTotals>                          ← Totals (see 7.5)
<WithholdingTax>…                         ← when set
```

### 7.4 Line
```
<LineNumber>            ← Line.LineNumber
<OrderReferences>?      ← Line.OrderReferences
<ProductCode>           ← Line.Product.ProductCode
<ProductDescription>    ← Line.Description (frozen == Product.ProductDescription)
<Quantity>              ← Line.Quantity rendered with fixed dp (see M2)
<UnitOfMeasure>         ← Line.Product.Unit
<UnitPrice>             ← Line.UnitPrice rendered via saftMoney (see R4)
<TaxBase>?              ← Line.TaxBase (omit when unused — none of 13 scenarios set it)
<TaxPointDate>          ← Line.TaxPointDate
<References>?           ← Line.References (NC/ND)
<DebitAmount|CreditAmount> ← LineSubtotal post-discount; family-keyed (see §7.6)
<Tax>                   ← dispatch on LineTax variant:
   VATTax     → <TaxType>IVA</…><TaxCountryRegion>…<TaxCode>…<TaxPercentage>…
   StampTax   → <TaxType>IS</…><TaxAmount>…
   NotSubjectTax → <TaxType>NS</…><TaxPercentage>0</…>
<TaxExemptionReason>?   ← VATTax.ExemptReason or NotSubjectTax.ReasonText
<TaxExemptionCode>?     ← Exemption code (M07 etc.)
<SettlementAmount>?     ← not used in v1 (no per-line settlement persisted)
```

### 7.5 DocumentTotals
```
<TaxPayable>            ← Totals.TaxTotal + Totals.StampDuty
<NetTotal>              ← Totals.NetTotal
<GrossTotal>            ← Totals.GrossTotal
<Currency>?             ← SalesInvoiceFields.Currency (G3)
<Settlement>?           ← SalesInvoiceFields.Settlement (G2)
<Payment>?              ← FRPayments (FR only)
```

### 7.6 Family-level aggregates (computed in `Export`)
`Totals.NetTotal` is unsigned. The projector picks DebitAmount vs
CreditAmount per `DocumentType` from the issuer's perspective and folds into
family totals:

| DocumentType | Line amount | Family total |
|---|---|---|
| FT/FS/FR/ND | `CreditAmount` = NetTotal | `TotalCredit` |
| NC | `DebitAmount` = NetTotal | `TotalDebit` |
| GR/GT/GA/GC/GD | `CreditAmount` = NetTotal | `TotalCredit` |
| OR/PF/NE/CM/FC/FO | `CreditAmount` = NetTotal | `TotalCredit` |
| RC/RG | per `PaymentLine.Movement` | `TotalDebit` / `TotalCredit` |

`NumberOfEntries` = doc count per family. `MovementOfGoods` additionally
needs `NumberOfMovementLines` = Σ `len(Lines)` and `TotalQuantityIssued` =
Σ quantities.

### 7.7 Encoding
1. Marshal AuditFile with `encoding/xml` to a `bytes.Buffer` (UTF-8).
2. Replace leading `<?xml version="1.0" encoding="UTF-8"?>` with
   `<?xml version="1.0" encoding="Windows-1252"?>`.
3. Pipe through `charmap.Windows1252.NewEncoder().Bytes(...)`. This cannot
   error in practice — every text-typed VO was Win-1252-validated at
   construction by `enforceWindows1252`.

---

## 8. Execution phases

### Phase A — Plumb gaps (domain + cmd)

Apply G1–G3 (G4–G6 are projector-side, handled in later phases):

- **G1 — store redesign.** Replace `memoryStore.docs` with the four typed
  maps shown in §4. Swap `c.store.record(doc.IssuedDocument)` for typed
  recorders in each `issueX` helper; add `recordPayment` and call it from
  `issuePayment`.
- **G2 — Settlement field.** Add `Settlement *Money` to
  `SalesInvoiceFields`. Replace `printSettlementSimulation` in 5.7 with a
  draft assignment.
- **G3 — Currency field.** Add `Currency *Currency` to `SalesInvoiceFields`.
  Replace `printCurrencySimulation` in 5.8.

Verify: `go test ./domain/...` green, `go run ./cmd` completes 13 scenarios.

### Phase B — Skeleton package

B1. Create `saft/` with empty files per §5. Add package doc in `doc.go`.
B2. `Export` returns `[]byte{}` and `nil` initially.
B3. `main.go` calls `saft.Export(...)` after `scenario513`, writes to
`out/SAFT-DEMO-2026-05.xml`. File ends up empty — wiring proven.

### Phase C — Header + MasterFiles

C1. Implement `NewHeader`, `Header` XML struct tags, encode just the header.
C2. Implement `Customer` and `Product` dedupe + marshal. Run; manually
inspect output.
C3. Implement `TaxTable` walk over `domain.taxRates` + exemption codes
observed in the doc set.
C4. `xmllint --noout --schema SAFTPT_1_04_01.xsd out/...xml` (operator
provides XSD locally).

### Phase D — SalesInvoices

D1. Marshal one `SalesInvoice` (5.1) and validate.
D2. Add cancellation handling (5.2 — Status="A" with Reason).
D3. References (5.5 NC, 5.13b ND) + OrderReferences (5.4 FT).
D4. Multi-tax breakdown (5.6) — four lines, four distinct tax rows.
D5. Discount (5.7) + Settlement block.
D6. Currency block (5.8).
D7. Anonymous customer / FS retail (5.1, 5.9, 5.10).
D8. FR payment block (5.13a).
D9. Family aggregates: `NumberOfEntries`, `TotalDebit`, `TotalCredit`.

### Phase E — MovementOfGoods

E1. GR valued (5.11a) — ShipFrom/ShipTo, MovementStartTime.
E2. GR non-valued (5.11b) — `UnitPrice=0`, `Tax` element omitted.
E3. GT/GA/GC/GD (5.13 transport variants).
E4. Aggregates: `NumberOfMovementLines`, `TotalQuantityIssued`.

### Phase F — WorkingDocuments

F1. NE (5.3), OR (5.12), PF/CM/FC/FO (5.13).
F2. NE post-MarkBilled (Status="F", BilledByInvoice reference omitted from
XSD — only Status changes).
F3. Aggregates.

### Phase G — Payments

G1. RC (5.13 RC) — settlement of FT 5.6, PaymentMethod block.
G2. RG (5.13 RG) — advance with no specific invoice.
G3. Aggregates.

### Phase H — Encoding finalize

H1. Wire UTF-8→Win-1252 transcoder.
H2. Rewrite XML declaration.
H3. Re-run xmllint with `--encoding Windows-1252`.

### Phase I — Golden test

I1. `saft/export_test.go`: run a synthesized 13-scenario doc set through
`Export`, diff against `testdata/scenarios_golden.xml`.
I2. Hash chain check: walk `SalesInvoices/Invoice[i+1]/Hash` and verify the
canonical input recomputes via `stubSigner`. Sanity check on cert chain
integrity through the projector.

---

## 9. Test strategy

- **Unit (per-element):** `header_test.go`, `masterfiles_test.go`,
  `sales_test.go`, etc. Each takes a hand-built minimal input and asserts on
  the marshalled fragment.
- **Golden file:** one big file from main's 13 scenarios. CI runs xmllint
  against XSD if XSD is checked in (or fetched in CI). Diff against the
  golden uses a normalization pass first (`xmllint --c14n11` or strip
  inter-element whitespace + sort attributes) — `encoding/xml` output
  formatting varies between Go releases and would cause spurious diffs.
- **Round-trip:** unmarshal the golden file back through Go's `encoding/xml`
  into the same structs; assert equality. Catches asymmetric struct tags.
- **Win-1252 byte check:** read 10 bytes after the XML declaration, assert
  no `0xC2`/`0xC3` UTF-8 continuation bytes; assert presence of the expected
  Win-1252 bytes for `ç`, `ã`, `á` (scenarios use Portuguese names).
- **No new fuzz tests** — projector is pure deterministic marshalling.

---

## 10. Risks and open questions

| # | Risk | Mitigation |
|---|---|---|
| R1 | Hand-written structs drift from XSD between revisions | Comment each struct with the XSD element name + line in `SAFTPT_1_04_01.xsd`. CI xmllint catches regressions. |
| R2 | Anonymous customer billing address may fail XSD min-occurs | Test early in Phase C with xmllint; if rejected, emit a stub address (`Desconhecido` country, blank required fields filled with `Desconhecido`). |
| R3 | `encoding/xml` self-closes empty elements; some validators dislike `<Foo/>` | Use `chardata + ""` workaround on the few elements that explicitly require open/close pairs (none expected in MVP). |
| R4 | `Money.MarshalJSON` emits integer cents; SAF-T wants `123.45` | Wrap in a projector-local type — `type saftMoney domain.Money` with `MarshalXML` emitting `Format2DP()`. Use `saftMoney` in every projector struct field; never let `domain.Money` marshal directly. |
| R5 | `Quantity` uses 5-decimal scale; SAF-T allows up to 3 dp by convention | Same pattern: `saftQty` wrapper with `MarshalXML` printing `%.4f` (trim trailing zeros). Demo quantities are integral so output is `1.0000` → `1`. |
| R6 | NotSubjectTax lines miss Breakdown | Out of scope: no scenario uses NS. Track as TODO referencing `document.go:103`. |
| R7 | SAF-T XSD uses `xs:sequence` everywhere — element order is strict | Every projector struct mirrors the XSD element order exactly. Comment each struct with the XSD type name + line number. Add a `xmllint --noout --schema` step to the test target so ordering regressions fail loud. |
| R8 | Go `encoding/xml` handles default namespaces awkwardly — emits redundant `xmlns=""` on children or duplicates the declaration | Marshal with a struct prefix (`saft:` prefix or explicit `xml:"AuditFile"` on root), then post-process the buffer: strip `xmlns=""` attributes, ensure exactly one `xmlns="urn:OECD:StandardAuditFile-Tax:PT_1.04_01"` on the root. Add a unit test asserting the byte pattern. |
| L1 | `stubSigner` uses `prev + "|" + canonical`, not AT's `<prev>;<canonical>` | Hash chain *shape* validates through the projector; AT verifier would reject. Acceptable v1 concession (signing is Tier-3). Document in §2. |
| L2 | `MaxLenCancellationReason` may exceed SAF-T `Reason` 50-char cap | Verify the constant during Phase D2; if too high, truncate in the projector with a logged warning. |
| Q1 | Does AT cert demand a `.zip` wrapper or just `.xml`? | [CONFIRMAR] — out of scope for v1 either way; trivial post-process. |
| Q2 | Should `ProductGroup` be populated from `Product.ProductGroup`? | Yes if set, omit otherwise. Fixtures don't set it — defer. |

---

## 11. Acceptance criteria

`go run ./cmd` creates `out/` (via `os.MkdirAll`) and writes
`out/SAFT-DEMO-2026-05.xml` such that:

1. `xmllint --noout --schema SAFTPT_1_04_01.xsd out/SAFT-DEMO-2026-05.xml` exits 0.
2. The file contains exactly:
   - 1 `<Header>`
   - 4 `<Customer>` entries (CustWithNIF, CustNoNIF1, CustNoNIF2, CustForeign)
   - 6 `<Product>` entries
   - SourceDocuments: 11 `<Invoice>` (FT×5 + FS×3 + NC×1 + ND×1 + FR×1),
     6 `<StockMovement>` (GR×2 + GT/GA/GC/GD),
     6 `<WorkDocument>` (NE + OR + PF + CM + FC + FO),
     2 `<Payment>` (RC + RG).
3. The cancelled FT (scenario 5.2 — first issuance in the FT series):
   `Invoice[InvoiceNo="FT FT2026/1"]/DocumentStatus/InvoiceStatus` == `A`
   with a `Reason` element present.
4. `WorkDocument[…NE…/1…]/DocumentStatus/WorkStatus` == `F` (NE billed by 5.4).
5. Hash chain across all sales+stock+work documents within each series
   verifies via `stubSigner.Sign` reconstruction.
6. No domain unit test regresses (tests touching `SalesInvoiceFields`
   zero-value comparisons may need a one-line update for the new
   `Settlement`/`Currency` fields — acceptable).

---

## 12. Order of file changes (PR-by-PR breakdown if split)

If shipped as one PR — single commit acceptable. If split:

- **PR1 (domain + store plumbing):** Phase A. Adds Settlement/Currency
  fields + redesigns `memoryStore` to four typed maps + wires recording.
  ~80 LOC.
- **PR2 (projector skeleton):** Phase B + C. Header + MasterFiles +
  `saftMoney`/`saftQty` wrappers + Win-1252 transcode stub. ~250 LOC.
- **PR3 (sales projection):** Phase D. ~350 LOC + per-scenario xmllint.
- **PR4 (movement + working + payments):** Phases E/F/G. ~250 LOC.
- **PR5 (encoding finalize + golden):** Phases H/I. ~100 LOC + testdata file.

---

## 13. Prerequisites before coding

Resolve Q1 (`.zip` wrapping) and R2 (anonymous customer billing address),
then drop `SAFTPT_1_04_01.xsd` somewhere committable so CI `xmllint` can run
offline. Phase A then takes ~30 min, Phase B lands same day.
