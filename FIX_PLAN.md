# Fix Plan — Derived from `GAP_ANALYSIS.md` + `AT_FEEDBACK.md`

> Executable plan to bring `domain/` to AT-certification-ready state, ordered
> so each step unblocks the next. Identifiers (T0-N, L-N, F-...) trace back
> to `GAP_ANALYSIS.md` and `AT_FEEDBACK.md`. Pair them with PR titles when
> implementing.
>
> **Authority order when specs conflict:** AT inspector feedback
> (`AT_FEEDBACK.md`) > SAF-T XSD > Portarias > `regras.md` > AUDIT.

**Working assumption:** every PR keeps `go test ./domain` and `go build ./...`
green. Public API may break — there are no external consumers yet — but each
PR carries a migration note inside its own header comment.

---

## 0. Goals and non-goals

### In scope
- Eliminate the T0 defects that block certification at the domain layer.
- Bring the field-level model in line with `regras.md` (Tier 1).
- Wire per-document-type invariants from `regras.md` §7 (Tier 2).
- Land the seams (interfaces, hooks) that Tier 3 modules will plug into.

### Out of scope
- SAF-T XML projector implementation (Tier 3 module #1). The breakdown,
  scale, and encoding *seams* are in scope; the XML writer is not.
- QR, PDF, AT webservice clients (Tier 3 modules #2, #3, #4).
- Persistence and audit log (Tier 3 modules #11–#15).
- Anything marked **[CONFIRMAR]** in `regras.md` whose value is unknown
  today; those land as configuration inputs, not constants.

---

## 0.5 Pre-flight checklist

Settle these before opening the first PR. If an item can't be settled, ship with the documented fallback (each item lists one) and flag the remaining uncertainty in code with a `TODO(...)` referencing this plan.

- [ ] **D-1 Money scale policy ratified** (Policy A vs B in §1). Fallback: Policy B.
- [ ] **`golang.org/x/text` dependency approved** for Win-1252 mapping (`encoding/charmap.Windows1252`). Fallback: hand-coded table sourced from `unicode.org/Public/MAPPINGS/VENDORS/MICSFT/WINDOWS/CP1252.TXT`.
- [ ] **`time/tzdata` import location decided** (suggestion: blank import in `domain/init.go` or `cmd/main.go`). Fallback: rely on host OS tzdata; document deployment requirement.
- [ ] **ATCUD code alphabet authoritative source located.** Fallback:
  permissive default (length ≥ 8, uppercase `[A-Z0-9]`), with a `TODO(atcud-alphabet)` comment and an `[CONFIRMAR]` entry in `legal_params.go`.
- [ ] **SAF-T (PT) v1.04_01 XSD obtained.** Drives `MovementStatus T`
  reading (§9) and the payment-hash question (D-6). Fallback if absent: adopt AT public-docs reading (T = "Por conta de terceiros" = ThirdParty), keep current naming, close as decided.
- [ ] **FS limits provided by legal.** Fallback: ship P2.9 with an
  `FSLimits{}` value of `{Retail: 1000_00, Default: 100_00}` (cents); mark `[CONFIRMAR]`.
- [ ] **Cancellation-deadline rule confirmed** (day 5 of month+1 23:59:59 Europe/Lisbon — currently asserted but not verified). Fallback:
  ship P2.6 with the day-5 rule as the default and a `[CONFIRMAR]` flag.

---

## 1. Pre-flight decisions

These must be settled BEFORE Phase 1 begins. Each is small but each
constrains a lot of downstream work.

### D-1 — `Money` scale and rounding policy

Two valid policies; pick one explicitly before P1.6.

**Policy A — store at cents (scale = 100).** Money becomes integer cents;
Quantity gets its own scale = 1_000 (3 decimals). `Mul` denominator
becomes the Quantity scale. Cleanest output (wire = storage); largest
blast radius (touches `Mul`, `MulPercent`, `roundDiv`, and every test
fixture that constructs Money).

**Policy B — keep shared scale = 100_000; enforce cent precision at the
boundary (RECOMMENDED).** `NewMoney(0.005)` returns
`ErrSubCentPrecision`; `MarshalJSON` emits integer cents from the int64;
`canonicalHashInput` formats from int64 cents directly (never via
`Float64()`). Quantity stays at the shared scale. Lower blast radius:
no test fixture using whole euros or cents-precision values changes;
only the JSON shape and signing path change.

**Rationale:** SAF-T mandates 2 decimals on serialization (`regras.md`
R-G3, I-F4). Storing at the same scale as the wire format (Policy A) is
structurally cleaner but violates the explicit "shared scale" design
contract in `money.go:13`. Karpathy rule: simpler wins; Policy B
delivers the same correctness with one-tenth the diff.

**Affected (Policy B):** `money.go`, `money_test.go`,
`issued_document.go::canonicalHashInput`, JSON-roundtrip fixtures.
**Affected (Policy A):** above plus `currency.go`, `document.go::CalculateTotals`,
`document_line.go::LineSubtotal`, every `NewMoney` call site.

### D-2 — How `SourceBilling` enters `Issue`
**Decision:** introduce an `IssueOptions{SourceBilling SourceBilling}`
struct passed to `Issue` and each family-specific issuer. Zero value
defaults to `SourceBillingProduced ("P")`.

**Note:** no `StatusOverride` field — `Status` is always derived (D-3).
Cancellation is a separate method (P2.6). Adding an override would be
YAGNI per Karpathy §2.

**Rationale:** keeps `Issue` arity manageable; lets the caller declare
`I` (integrated) or `M` (manual) intent at the call site.

### D-3 — `Status` derivation rule
**Decision:** compute inside the family issuer, in this priority:
1. `StatusSelfBilled ("S")` if family = sales and `SpecialRegimes.SelfBilling`.
2. `StatusThirdParty ("T")` if family = transport and
   `SpecialRegimes.ThirdParties` (pending §9 verification of MovementStatus
   T semantics — fallback in §0.5).
3. `StatusNormal ("N")` otherwise.

Cancellation (`"A"`) is set by `IssuedDocument.Cancel(...)` (P2.6). Billed
(`"F"`) for working documents is set by a separate transition method
(P2.15). `Issue` never sets `A`, `F`, or `R`.

### D-4 — Win-1252 validation boundary
**Decision:** check at the **constructor of every text-typed VO** that
maps to an exportable SAF-T field. Implement a single
`enforceWindows1252(s string, field string) error` helper used by:
`Address.*` constructors, `Company.*`, `Customer.*`, `Product.*`,
`DocumentLine.Description` (new), free-form fields on `LineTax`,
exemption text. NOT applied to internal-only strings (UUIDs, enum
values).

**Dependency:** `golang.org/x/text/encoding/charmap.Windows1252` provides
the authoritative mapping. Add to `go.mod` as part of P1.5. Fallback in
§0.5 if dep cannot be approved.

**Rationale:** regras R-G7 "comprimento e formato no construtor, não na
serialização." Illegal state never instantiated.

### D-5 — Tax-breakdown shape
**Decision:** `TaxBreakdown` is a sorted slice of entries; internal
accumulation uses a map keyed by `(TaxRegion, TaxCategory, ExemptionCode)`.
Entry: `{Region, Category, ExemptionCode (optional), Base, Tax,
ExemptionDescription (optional)}`.

**Rationale:** SAF-T DocumentTotals and QR I/J/K both consume ordered output.
Map is for internal grouping only.

### D-6 — Cash-VAT payment signing (D-RC-2, D-RC-3)
**Decision:** DEFER fully until §9 question 2 is settled with the AT
XSD revision. No `Series.CashVATRegime` field, no plumbing through
`SpecialRegimes.CashVAT` to the payment path. Drop a single
`// TODO(cash-vat-signing): see FIX_PLAN.md §1 D-6, regras §7.6` in
`IssuePayment`. When the XSD answer arrives, design the data flow then.

**Rationale:** adding state we won't use violates Karpathy §2 ("no
flexibility not requested"). Pretending we know the canonical input
format is worse than the bug.

### D-7 — Exemption-codes reference table
**Decision:** keep the existing Go constants for `M01..M99` and the
existing `exemptionDescriptions` map. No refactor in this plan.

When date-versioned data lands (separate feature), the change is local:
introduce a `Resolve(at time.Time) (Entry, error)` API and migrate the ~5
call sites then. Designing the seam today is YAGNI per Karpathy §2.

**Open thread:** regras §10 versioning remains a deferred item; track in
`GAP_ANALYSIS.md` Tier-3 #7.

### D-8 — `IssueOptions.Now` injection vs caller-passed `time.Time`
**Decision:** keep `now time.Time` as a positional argument (already
present). All TZ normalization happens INSIDE `Issue`. Callers may pass
any clock value; `Issue` converts to Europe/Lisbon before signing /
storing. The `Clock` interface introduced in Phase 1.5 (§2.5) is for
deadline computations in `Cancel()`, not for `Issue`.

---

## 2. Phase 1 — Foundational defects

8 PRs, ordered so each is independently mergeable but the cumulative
effect closes the Tier-0 list. Most are small; T0-1 and T0-2 are the
heavy ones.

### P1.1 — T0-3 Madeira region rename (smallest, lowest risk)
- **Files:** `domain/tax.go`, `domain/line_tax.go`, all golden fixtures
  in `domain/*_test.go`.
- **Approach:** rename Go identifier `PTAM` → `PTMA`; change value
  `"PT-AM"` → `"PT-MA"`; re-key `taxRates`; sweep tests; verify
  `TaxJurisdiction.IsValid` accepts the new value (it already does).
- **Acceptance:** `domain.GetTaxRate(domain.PTMA, domain.TaxNormal, "")`
  returns the Madeira normal rate; old `domain.PTAM` no longer compiles.
- **Test:** add `TestGetTaxRate_Madeira` if not present.
- **Deps:** none.
- **Effort:** XS.

### P1.2 — T0-5 monotonic date guard
- **Files:** `domain/issued_document.go::validateIssueContext`.
- **Approach:** if `series.LastSystemDate != nil` and
  `draft.Date.Before(*series.LastSystemDate)` → error.
- **Acceptance:** issuing a doc with `Date` before the prior issue's
  system date fails with `errors.Is(err, ErrDateRegression)` (new
  sentinel in `errors.go`).
- **Test:** `TestIssue_RejectsBackDated` — issue once, then attempt to
  issue with earlier date.
- **Deps:** none.
- **Effort:** S.

### P1.3 — T0-6 ATCUD code validation **[CONFIRMAR alphabet]**
- **Files:** `domain/atcud.go` (add `ValidateATCode(string) error`),
  `domain/series.go::RegisterWithAT`, `domain/atcud_test.go`, plus the
  test helper `registeredSeries(...)` (likely in
  `domain/issued_document_test.go` or a shared test helper file) — its
  AT-code argument must satisfy the new validator.
- **Approach:** validate length ≥ 8 and alphabet. **The exact alphabet
  is [CONFIRMAR]** against AT's authoritative reference (see §0.5
  fallback). Two candidate rules in circulation:
  - Permissive: uppercase `[A-Z0-9]`, length ≥ 8.
  - Strict (per AUDIT 1.3): consonants + digits 2–9
    (`[BCDFGHJKLMNPQRSTVWXZ2-9]`), case-insensitive.
  Ship the permissive default to avoid rejecting valid codes; tighten
  if AT documentation says otherwise.
- **Acceptance:** under permissive rule, `RegisterWithAT("AAJ7H5K2", ...)`
  succeeds (length 8, alphanumeric uppercase); `RegisterWithAT("aaj7", ...)`
  fails (length < 8). Update assertions if rule tightens.
- **Test:** table-driven valid/invalid cases.
- **Deps:** none. Sweep test fixtures (especially `registeredSeries`)
  for codes that violate the chosen rule and replace.
- **Effort:** S.

### P1.4 — T0-7 LineNumber auto-assignment + uniqueness
- **Files:** `domain/document.go::AddLine` (if exists — verify; otherwise
  add), `CommonDraftDocument.Validate`.
- **Approach:** `AddLine` assigns `LineNumber = len(d.Lines) + 1`,
  ignoring caller value; `Validate` scans for duplicates as a defense.
- **Acceptance:** appending three lines produces 1, 2, 3 regardless of
  caller-set values; constructing with two identical `LineNumber`
  literals fails `Validate`.
- **Test:** `TestAddLine_AutoSequences`, `TestValidate_RejectsDuplicateLineNumber`.
- **Deps:** none.
- **Effort:** S.

### P1.5 — T0-8 Windows-1252 boundary validator
- **Files:** new `domain/win1252.go` with `enforceWindows1252`; thread
  into VO constructors per D-4.
- **Approach:** use the Win-1252 mapping table (256 chars); rejecting
  any rune not representable. Helper is pure-function; no I/O.
- **Acceptance:** constructing an `Address` with `City = "北京"` fails
  with a clear error naming the offending rune; `"Lisboa"` succeeds;
  `"São Paulo"` succeeds (`ã` is in Win-1252).
- **Test:** table-driven; cover all VOs touched.
- **Deps:** none.
- **Risk:** false positives if mapping table is wrong. Use the
  authoritative table from the unicode.org mappings dir; cite source in
  the file header comment.
- **Effort:** M.

### P1.6 — T0-1 Money rounding policy (per D-1 outcome)
- **Decision:** Policy A or Policy B per D-1. The steps below assume
  **Policy B** (recommended); under Policy A the file list expands and
  arithmetic helpers change too.
- **Files (Policy B):** `money.go`, `money_test.go`,
  `issued_document.go::canonicalHashInput`, `json_roundtrip_test.go`
  fixtures.
- **Approach (Policy B):**
  1. `NewMoney` rejects sub-cent input: return `ErrSubCentPrecision` if
     `math.Round(euros*scale) % 1000 != 0` (since scale=100_000, one
     cent = 1_000 internal units).
  2. Replace `Money.MarshalJSON` with integer-cents output:
     `json.Marshal(int64(m) / 1000)`.
  3. `UnmarshalJSON` accepts integer cents (and remains backward-
     compatible with float input during the transition by trying both,
     OR ship a one-shot migration — pick one and document).
  4. Add a `formatCents` helper that handles negative values correctly:
     ```go
     func formatCents(cents int64) string {
         sign := ""
         if cents < 0 { sign = "-"; cents = -cents }
         return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
     }
     ```
     Replace `strconv.FormatFloat(gross.Float64(), 'f', 2, 64)` in
     `canonicalHashInput` with `formatCents(int64(gross) / 1000)`.
  5. Drop overflow concern post-shift; `Mul`/`MulPercent` operate on the
     same int64s as before. (P2.14 becomes mostly cosmetic — see note
     there.)
- **Acceptance:** `NewMoney(49.50)` succeeds; `NewMoney(0.005)` returns
  `ErrSubCentPrecision`; `json.Marshal(NewMoney(49.50))` emits `4950`;
  round-trip preserves the value exactly; `canonicalHashInput` for
  `Money(-150)` (= -€0.015 — wait, sub-cent — rejected at construction;
  use `Money(-15000)` = -€0.15) formats as `"-0.15"`.
- **Test:** new `TestMoney_RejectsSubCent`,
  `TestMoney_MarshalCents`, `TestFormatCents_HandlesNegative`,
  `TestCanonicalHashInput_NoFloatPath`.
- **Deps:** P1.1 (Madeira rename touches some fixtures already).
- **Fixture regeneration:** `json_roundtrip_test.go` golden JSON
  changes (Money emits int instead of float). Regenerate as part of
  this PR; do NOT defer.
- **Risk:** the float→int JSON shape change breaks any external
  consumer; none today. Migration helper for old data (if any) is
  out-of-scope.
- **Effort:** M (Policy B) or L (Policy A).

### P1.7 — T0-4 Europe/Lisbon timezone normalization
- **Files:** `domain/issued_document.go::Issue`, `canonicalHashInput`,
  plus one new file for the tzdata import (e.g., `domain/init.go` or
  add the line to `cmd/main.go`).
- **Approach:**
  ```go
  import _ "time/tzdata"  // ensures Europe/Lisbon resolves on minimal images
  ...
  loc, err := time.LoadLocation("Europe/Lisbon")
  if err != nil { return ... }  // surface, don't ignore
  draftDateLisbon := draft.Date.In(loc)
  nowLisbon := now.In(loc)
  ```
  Use the Lisbon-adjusted values everywhere downstream (signing,
  `IssuedDocument.SystemEntryDate`, `Period` derivation). Don't swallow
  the `LoadLocation` error; if tzdata is missing the deploy is
  misconfigured and should fail loudly.
- **Acceptance:** issuing the same draft with `now = 23:59 UTC` and
  `now = 00:00 UTC` (next day) produces hashes computed against the
  Lisbon-local timestamp.
- **Test:** `TestIssue_NormalizesToLisbon`: two issuances differing only
  by caller TZ produce identical canonical input.
- **Fixture regeneration:** hash-chain test golden values change here
  too. Regenerate after this PR (combine with P1.6 regen if landed
  together).
- **Deps:** P1.6 (canonical input format change ordering).
- **Effort:** S.

### P1.8 — T0-2 TaxBreakdown aggregation
- **Files:** `domain/tax.go` (add `TaxBreakdown`, `TaxBreakdownEntry`),
  `domain/document.go::CalculateTotals`, `Totals` (add `Breakdown` field).
- **Approach:** during the line walk in `CalculateTotals`, accumulate
  into a map keyed per D-5. After the walk, sort entries deterministically
  (region asc, category asc, exemption code asc) into a slice. Surface on
  `Totals.Breakdown`.
- **Acceptance:** a document with two lines (PT/NOR + PT-AC/RED + PT/ISE
  with M04) produces three breakdown entries with correct base/tax/sums;
  totals reconcile (`Σ base == NetTotal`, `Σ tax == TaxTotal`).
- **Test:** `TestTaxBreakdown_AggregatesByRegionAndCategory`,
  `TestTaxBreakdown_ExemptLinesGroupByExemptionCode`,
  `TestTaxBreakdown_SumsMatchTotals`.
- **Fixture regeneration:** `json_roundtrip_test.go` Totals shape changes
  with the new `Breakdown` field. Regenerate.
- **Deps:** P1.1 (Madeira), P1.6 (Money scale), P1.7 (canonical input
  unchanged here but tests may share fixtures).
- **Risk:** the field addition is breaking for any caller comparing
  `Totals` structurally; there are none, but `omitzero` on the new
  field keeps JSON shape clean for empty cases.
- **Effort:** M.

**Phase 1 exit criteria:**
- All Tier-0 IDs closed.
- `go test ./domain` green, `go build ./...` green.
- JSON-roundtrip fixtures regenerated once (Money cents + Totals
  Breakdown both shift the shape).
- Hash-chain golden values regenerated once (canonical input shifts
  twice: P1.6 Money format + P1.7 Lisbon TZ).
- A short PHASE1_CHANGES.md note in the repo (not in this plan) listing
  the new error sentinels and any breaking signature changes.

---

## 2.5 Phase 1.5 — Seams introduced before Phase 2

Two interfaces gate Phase-2 work. Land them between Phase 1 and Phase 2
so consumers in Phase 2 don't pull plumbing changes into their PRs.

### P1.5a — `Clock` port
- **Files:** new `domain/clock.go`.
- **Approach:**
  ```go
  type Clock interface { Now() time.Time }
  type SystemClock struct{}
  func (SystemClock) Now() time.Time { return time.Now() }
  ```
- **Consumer:** P2.6 (`Cancel`) needs it for deadline math. `Issue`
  keeps the explicit `now time.Time` argument; only deadline-bearing
  operations take a `Clock`.
- **Effort:** XS.

### P1.5b — `IssuedDocumentReader` port
- **Files:** new `domain/store.go`.
- **Approach:**
  ```go
  type IssuedDocumentReader interface {
      FindByNumber(DocNumber) (IssuedDocument, error)
  }
  ```
- **Consumer:** P2.9 (FS limits don't need it), D-NC-1 (NC value
  cross-check) and D-RC-1 (RC outstanding cross-check) — both deferred
  to Phase 3 actual implementation, but the port lands now.
- **Risk:** over-design. Keep the interface to one method until a real
  consumer exists; resist adding speculative methods.
- **Effort:** XS.

---

## 3. Phase 2 — Model completeness

Each item is a separate PR; landable in roughly the listed order.
Dependencies named where they exist.

### P2.1 — Add `Description` to `DocumentLine` (L-1) **[revised by F-SAFT-9]**
- AT requires strict equality: line description **must equal** the
  product's `ProductDescription` at projection time. Open question §8
  Q-4 in `AT_FEEDBACK.md`: pick policy A/B/C before coding.
- **Recommended policy (B):** keep the field, copy at construction from
  `Product.ProductDescription`, and have the SAF-T projector
  re-emit the product-table value (ignoring any drift). Validate at
  issue time that the snapshot equals the current product table entry.
- `DocumentLine.Description string` (1..200), validated via D-4 helper.
- **Test:** `TestLine_DescriptionDefaultsFromProduct`,
  `TestLine_RejectsTooLong`,
  `TestIssue_RejectsDescriptionDriftFromProductTable` (F-SAFT-9 guard).

### P2.2 — Use `TaxBase` in `CalculateTotals` (L-2)
- In the line walk, if `line.TaxBase` is set, use it as the tax base
  instead of `LineSubtotal()`; apply discount per existing rule (regras
  R-L: discount on `LineSubtotal`; not on `TaxBase`).
- **Test:** `TestCalculateTotals_TaxBaseOverridesUnitPrice`.

### P2.3 — Extend `ProductType` enum (P-1)
- Add `ProductOther = "O"`, `ProductExcise = "E"`, `ProductParafiscal = "I"`.
- Update `Product.Validate` + tests.

### P2.4 — Extend `TaxCategory` enum (L-4)
- Add `TaxOther TaxCategory = "OUT"`.
- Update rate-table semantics (SAF-T `OUT` typically has no rate; treat as
  "tax declared in `OUT` is a fixed-amount block on the line").
- **Risk:** the canonical rate table assumes every category has rates per
  region. `OUT` does not. Add a discriminator branch.

### P2.5 — `IssueOptions` parameter (D-2 + AUDIT 1.7, 1.8; H-3)
- Introduce `IssueOptions{SourceBilling SourceBilling}` — no
  `StatusOverride` (see D-2).
- Family issuers: `IssueSalesInvoice(draft, series, signer, sourceID, now, opts)`,
  `IssueStockMovement(...)`, `IssueWorkDocument(...)`, `IssuePayment(...)`.
- Zero-value `IssueOptions{}` → `SourceBilling = "P"`.
- Derive `Status` per D-3 (always; not overridable).
- **Files:** `domain/issued_document.go`, `domain/sales_invoice.go`,
  `domain/stock_movement.go`, `domain/work_document.go`,
  `domain/payment.go`, plus the demo callsite `cmd/main.go`, plus all
  family tests in `domain/families_test.go`.
- Closes AUDIT 1.7, 1.8, and the broader `SpecialRegimes` propagation gap
  (§2.4 of `GAP_ANALYSIS.md` — covers SelfBilling, ThirdParties, and
  CashVAT-flag-not-propagated; CashVAT signing itself remains deferred
  per D-6).
- **Test:** `TestIssueSalesInvoice_SelfBillingPropagatesStatus`,
  `TestIssueStockMovement_ThirdPartiesPropagatesStatus`,
  `TestIssue_SourceBillingHonored`,
  `TestIssue_CashVATFlagRecordedNotSigned` (records the regime flag
  even though signing is deferred).

### P2.6 — `Cancel()` flow with `Reason` (H-2 + Tier-3 #6 partial)
- `IssuedDocument.Cancel(reason string, at time.Time, clock Clock) error`.
- **Depends on:** P1.5a (`Clock` port).
- Computes the e-Fatura deadline (day-5 of month+1, 23:59:59 Europe/Lisbon
  — **[CONFIRMAR]**, see §0.5 fallback).
- Within the deadline → set `Status = "A"`, `Reason`, `StatusDate = at`.
- After the deadline → return `ErrCancellationDeadlinePassed` (caller
  must use recovery flow, out of scope here).
- **Test:** `TestCancel_BeforeDeadline_Succeeds`,
  `TestCancel_AfterDeadline_Errors`,
  `TestCancel_DeadlineComputedInLisbon`.

### P2.7 — Mandatory `ShipFrom` / `ShipTo` on guias (G-1)
- `DraftStockMovement.Validate` rejects `nil` `ShipFrom` or `ShipTo`.
- Adjust existing tests; add `TestDraftStockMovement_MissingShipFromRejected`.

### P2.8 — `FR` payment block invariant (D-FR-1, D-FR-2) **[sharpened by F-SAFT-14]**
- Add embedded `Payment *FRPayment` on `DraftSalesInvoice` with fields
  matching SAF-T §4.1.4.20.6:
  - `PaymentMechanism` (enum: NU, MB, TB, CC, ...)
  - `PaymentAmount Money`
  - `PaymentDate time.Time`
- `DraftSalesInvoice.Validate`: if `DocumentType == FR`, `Payment != nil`
  and `Payment.PaymentAmount.Equal(Totals.GrossTotal)` after
  `CalculateTotals`. Multiple payment methods allowed; their sum must
  equal `GrossTotal`.
- **Test:** `TestDraftFR_RequiresPayment`,
  `TestDraftFR_AmountMatchesGross`,
  `TestDraftFR_MultipleMethodsSumToGross`.

### P2.9 — `FS` limit gate + relaxed customer (D-FS-1, D-FS-2)
- Introduce `AnonymousCustomer` constructor: accepts NIF only (optional);
  fills `CompanyName = "Consumidor final"`, omits address — but only
  legal for FS.
- `DraftSalesInvoice.Validate`: if `FS` and `GrossTotal > limit` →
  reject. The limit selection consults `EAC.IsRetailActivity` and
  whether `Customer` is `AnonymousCustomer` and all lines are
  `ProductType = "P"`.
- Limits configurable: pass `FSLimits{Retail, Default}` via construction.
  **[CONFIRMAR]** values; don't hardcode.
- **Test:** `TestDraftFS_OverLimitRejected`, `TestDraftFS_AnonymousOK`,
  `TestEAC_RetailActivity` (use the existing helper).

### P2.10 — `Series.Version` optimistic lock (S-1)
- Add `Version uint64` on `Series`; `Issue` increments on success.
- Domain stays single-threaded; infra layer compares `Version` on UPDATE
  to detect concurrent issuance.
- **Test:** `TestIssue_IncrementsSeriesVersion`.

### P2.11 — (deferred per D-7)
Exemption-codes versioning is not addressed in this plan. Tracked in
`GAP_ANALYSIS.md` Tier-3 #7. When date-versioned data ships, revisit.

### P2.12 — `WithholdingTax` → "amount payable" derived total (TT-2)
- Add `Totals.AmountPayable Money` derived as
  `GrossTotal − Σ WithholdingTax.Amount`. Surface on `Totals`.
- Feeds QR field `P` later.
- **Test:** `TestTotals_AmountPayable`.

### P2.13 — `Money.MarshalJSON` emits integer cents (TT-3)
- Folded into P1.6 (see P1.6 step 2). This item exists only as a
  pointer; no separate PR.

### P2.14 — `Money` overflow guards (TT-4) **[likely cosmetic post-P1.6]**
- Under Policy B (P1.6), the int64 arithmetic in `Mul`/`MulPercent` is
  unchanged, so the documented thresholds (~€920M / ~€9B) still apply.
- Under Policy A, the thresholds rise ~1000× and overflow is
  unreachable in practice.
- Scope: replace the panic in `MulPercent` with `ErrInvalidPercent`
  and add checked-multiply on `Mul`. Symbolic correctness, not a
  pressing bug.
- **Test:** `TestMoney_MulOverflowDetected`.

### P2.16 — `Currency.Amount` foreign-currency precision (M-1)
- `Currency.Amount` is currently typed `Money` (EUR-precision). For
  JPY (0 decimals) or KWD (3 decimals), the type is wrong.
- **Decision:** introduce `ForeignAmount{Cents int64, Precision uint8}`
  or document `Currency.Amount` strictly as the EUR-equivalent and
  forbid foreign-precision use. Pick before the SAF-T projector lands.
- **Test:** `TestCurrency_JPYAmount_NoFractional`,
  `TestCurrency_KWDAmount_ThreeDecimals`.

### P2.15 — Smaller polish (parallelizable; one PR each or batched)
- L-3 joint exemption code+text invariant on `VATTax.Validate`.
- L-5 `Quantity` precision policy (locked via P1.6).
- P-2 `ProductCode` immutability — add `Product.InUse bool` field,
  enforce in setters (if setters exist) or document the rule in
  `Product.Validate` if mutation is by struct replacement.
- C-1 reserved `Consumidor final` `CustomerID` constant +
  `Customer.IsAnonymous() bool`.
- C-3 PT postal-code format check (regex `^\d{4}-\d{3}$` when
  `Country == "PT"`).
- X-4 `DocReference.Reference` non-empty when used in NC/ND.
- X-5 `HashControl` regex tightening to real DocumentType prefixes.
- M-2 `WorkDocument.MarkBilled(invoiceNumber DocNumber) error` — sets
  `Status = "F"`, captures the referenced invoice.
- M-3 `Series.Year` semantics — add `Validate`: if non-zero, must equal
  `Date.Year()` at the time of first issuance. No auto-roll for now;
  document the open question.
- M-4 Validating UnmarshalJSON on all VOs with invariants —
  `Customer.UnmarshalJSON` shape-only today (`customer.go:19`); align
  with `Discount`'s pattern. Same audit pass on `Address`, `Company`,
  `Product`.
- AUDIT 3.5 `DeliveryIDs` length 255 → 200.
- AUDIT 3.16 `PaymentMethod.Mechanism` required.
- AUDIT 3.18 `LineTax.UnmarshalJSON` calls `Validate`.
- AUDIT 3.19 drop redundant `Validate` double-call in family issuers.
- AUDIT 3.13 `DraftDocument` interface — drop (no consumers).
- AUDIT 3.14 `ATCUD("0")` branch — remove (unreachable post-T0-6).

### P2.17 — `SoftwareIdentity` value type (F-NEW-1)
- New `domain/software_identity.go`:
  ```go
  type SoftwareIdentity struct {
      ProducerTaxID     TaxID  // e.g., 519348761 (Faturly producer)
      SoftwareName      string // e.g., "Faturly"
      ProducerName      string // e.g., "AVENIDA DO CODIGO ..."
      Version           string // e.g., "1.0.0" — must match Modelo 24
      CertificateNumber string // e.g., "9999" — AT-assigned
  }
  ```
- Single instance per build; loaded at startup. Used by SAF-T Header
  (`ProductCompanyTaxID`, `ProductID`, `ProductVersion`) and QR field
  `R`. **Distinct from `Company` (which is the issuer's identity).**
- Closes F-SAFT-4, F-SAFT-5, F-SAFT-6.
- **Test:** `TestSoftwareIdentity_ProductIDFormat`,
  `TestSoftwareIdentity_VersionImmutable`.

### P2.18 — Remove `DC` document type (F-NEW-2)
- Drop from `DocumentType` enum, `docTypeRules`, and the working-family
  list.
- **Test:** `TestDocumentType_DCRejected`.
- **Migration concern:** if any persisted DC documents exist (none in
  this codebase yet), they must be migrated before this lands.

### P2.19 — `PaymentTerms` (due date) on Totals (F-NEW-4)
- Add `PaymentTerms *time.Time` to `Totals` (or
  `CommonDraftDocument`).
- SAF-T projector emits `DocumentTotals.Settlement.PaymentTerms` when
  set.
- **Test:** `TestTotals_PaymentTermsRoundtrip`.
- Closes F-SAFT-12.

### P2.20 — Five-working-day emission guard (F-NEW-5)
- Add `HolidayCalendar interface { IsHoliday(date time.Time) bool }`.
- Helper: `workingDaysBetween(start, end time.Time, cal HolidayCalendar) int`.
- In `Issue`: reject if working-day gap > 5 (CIVA Art. 36.º).
- Ship with weekend-only fallback (`EmptyCalendar`); `[CONFIRMAR]` on
  the production holiday source.
- Recovery-flow entries (P2.24) bypass this guard.
- **Test:** `TestIssue_RejectsLateEmission`,
  `TestWorkingDays_SkipsWeekends`,
  `TestRecovery_BypassesLateEmissionGuard`.
- Closes F-SAFT-13.

### P2.21 — `Currency.Date` (rate-at-issuance) (F-NEW-6)
- Add `Date time.Time` to `Currency`; require
  `Currency.Date.Equal(draft.Date)` at issuance.
- **Test:** `TestCurrency_RateDateMustMatchInvoiceDate`.
- Closes F-SAFT-15.

### P2.22 — `MovementStartTime ≥ SystemEntryDate` guard (F-NEW-7)
- In `IssueStockMovement`, after `SystemEntryDate` is set, reject if
  `MovementStartTime.Before(SystemEntryDate)`.
- **Test:** `TestIssueStockMovement_RejectsStartBeforeSystemEntry`.
- Closes F-SAFT-16.

### P2.23 — Valued vs non-valued guides (F-NEW-8)
- Add `Valued bool` to `DraftStockMovement` (or derive: any line with
  non-zero `UnitPrice` ⇒ valued).
- Non-valued path: lines without `Tax`; projection emits `I1=0` on QR.
- **Test:** `TestStockMovement_NonValuedNoTax`,
  `TestStockMovement_ValuedRequiresTax`.
- Closes F-SAFT-18.

### P2.24 — Recovery flows (F-NEW-13)
- **Depends on:** P1.5b (`IssuedDocumentReader`), P2.20 (so recovery
  can opt out of the 5-day rule).
- Two new entry points in `domain/recovery.go`:
  - `IntegrateManualDocument(input ManualDocumentInput, series *Series, signer Signer, opts IssueOptions, now time.Time) (IssuedDocument, error)`
  - `IntegrateBackupDocument(input BackupDocumentInput, series *Series, signer Signer, opts IssueOptions, now time.Time) (IssuedDocument, error)`
- Both set `SourceBilling = "M"` and use a `HashControl D-form` per
  Portaria 363/2010 (`{key}-XXD M {series}/{n}` shape).
- Both bypass F-NEW-5 and the monotonic-date guard (recovery may
  restore documents out of order).
- **Test:** the two AT exercises from §5 in `AT_FEEDBACK.md` —
  manual series F #23 (2026-01-02), series D #3 (2026-01-01).
- Closes F-REC-1, F-REC-2.

### P2.25 — ND product-set validation (F-NEW-9)
- **Depends on:** P1.5b.
- In `DraftSalesInvoice.Validate` when `DocumentType == ND`:
  - For each `Line.References[i]`, fetch the originating invoice via
    `IssuedDocumentReader`.
  - Reject if any line `ProductCode` is not present on any referenced
    invoice.
  - Reject if any line `Quantity` differs from the originating
    quantity (ND only corrects values).
- **Test:** `TestDraftND_RejectsNewProduct`,
  `TestDraftND_RejectsQuantityChange`.
- Closes F-SAFT-19.

### P2.26 — Frozen QR payload at issuance (F-NEW-10)
- Add `QRPayload string` to `IssuedDocument`. Compute at issue time
  using the issuance-time status. Reprint reads this verbatim.
- Domain-only step lands here; QR-string composition logic lives in
  the QR generator module (Tier 3), but the field reservation lands
  now so future reprint reads from `IssuedDocument` directly.
- **Test:** `TestIssue_StoresQRPayload`,
  `TestCancel_DoesNotMutateQRPayload`.
- Closes F-QR-3.

**Phase 2 exit criteria:**
- All Tier-1 + Tier-2 IDs closed except those depending on confirmation
  (`[CONFIRMAR]` items, cash-VAT signing, NC/RC cross-doc rules).
- All `F-SAFT-*` and `F-NEW-*` items in `AT_FEEDBACK.md` §7 closed
  (excludes pure print/QR concerns — those land with the Tier-3
  modules).
- `go test ./domain` green; coverage of new invariants ≥ 90%.
- Round-trip JSON tests (`json_roundtrip_test.go`) pass for every
  document family with the new fields.

---

## 4. Phase 3 — Modules (out of this plan's depth; seams only)

Land seams in `domain/` so Phase-3 modules can plug in without
re-opening Phase-1/2 code. Concrete module work belongs to a separate
plan.

### Seams that already exist after Phase 1.5 + Phase 2
- **`Clock` port** — landed in P1.5a. Consumer: P2.6.
- **`IssuedDocumentReader` port** — landed in P1.5b. Consumers
  (deferred): NC-value-vs-referenced (D-NC-1) and RC-outstanding
  (D-RC-1) checks.

### Seams to leave as doc files (no Go code yet)
- **SAF-T projector contract** — a `domain/saft.go` header comment
  describing the expected input shape (`Totals.Breakdown`, line
  enumeration, snapshot fields). The projector implementation lives
  outside `domain/`.
- **QR contract** — same: a header comment in `domain/qr.go` listing
  the QR fields and which `IssuedDocument` field feeds each.

### Out of scope here
- SAF-T XML writer + Win-1252 encoder.
- QR generator.
- PDF render layer + menções obrigatórias.
- AT webservice clients (ATCUD, WDT, e-Fatura).
- Persistence + retention + audit log + idempotency.
- Recurring / reminders / subscriptions.

---

## 5. Cross-cutting workstreams (run alongside, not blocking)

### W-1 Test hygiene
- `TestIssue_HashChainDependsOnPrevHash` (AUDIT 3.20) — issue two
  documents with the same canonical body but different `prevHash`;
  assert distinct outputs.
- `TestValidate_Idempotent` — calling `CommonDraftDocument.Validate()`
  twice on the same value doesn't mutate it (sanity check after
  P1.4 LineNumber auto-assignment).
- `TestIssue_RejectsBackDated_RelativeToSeries` — second issuance with
  earlier `draft.Date` than `series.LastSystemDate` fails (covers P1.2
  invariant; pair with AUDIT 3.22).
- `TestCalculateTotals_RoundingClosure` — sum of per-line tax (already
  rounded) equals the declared `Totals.TaxTotal` exactly.
- Golden-hash fixtures regenerated once per Phase 1 (P1.6 + P1.7 all
  change the canonical bytes; P1.8 changes Totals JSON shape).

### W-2 Error sentinels
- All new errors land in `domain/errors.go` as exported sentinels.
- Wrap with `fmt.Errorf("...: %w", err, sentinel)` for context.

### W-3 Naming consistency
- Use `New<Type>` for constructors that validate; `<Type>{}` literal
  initialization is forbidden by convention for any VO with invariants.
- `Validate()` always returns `error`, never `bool`.

### W-4 [CONFIRMAR] tracking
- Every `[CONFIRMAR]` from `regras.md` that lands in code does so as a
  configurable input on the constructor or as a `var` documented as
  "must be loaded from current legislation; see regras.md §N".
- Maintain a small `domain/legal_params.go` collecting these inputs as
  a single struct.

---

## 6. Risk register

| Risk | Impact | Mitigation |
|---|---|---|
| Money JSON shape change breaks roundtrip fixtures | Red `json_roundtrip_test.go` | Regenerate fixtures inside P1.6's PR; do NOT defer the regen. |
| Win-1252 mapping table off-by-one | False rejections | Use `golang.org/x/text/encoding/charmap.Windows1252`; if rolling our own, cite `unicode.org/Public/MAPPINGS/VENDORS/MICSFT/WINDOWS/CP1252.TXT` in the file header. |
| `MovementStatus T` semantics still unresolved | Wrong SAF-T export | §0.5 fallback: adopt AT public-docs reading (T = ThirdParty), keep code as-is, close the audit item with a citation. Do NOT leave the thread open indefinitely. |
| Cash-VAT signing implemented prematurely | Wrong canonical input | D-6: defer fully; no data-flow code lands until XSD revision confirmed. |
| Repository port over-designed | Coupling | P1.5b: one method only (`FindByNumber`); add more only when a real consumer exists. |
| Phase 1 PRs land in parallel and conflict on fixtures | Merge conflicts | P1.1, P1.2, P1.3, P1.4, P1.5 can land in parallel (independent files). P1.6, P1.7, P1.8 must land serially in that order (each touches signing or totals JSON). |
| Breaking public API | Downstream churn | No external consumers today; document changes in each PR header. |
| `time.LoadLocation("Europe/Lisbon")` fails on minimal Docker image | Production outage on first deploy | P1.7 imports `_ "time/tzdata"` and surfaces the `LoadLocation` error explicitly. |
| ATCUD alphabet rule turns out stricter than shipped | Real codes rejected | Ship permissive default; tighten when AT reference confirmed (§0.5). |

---

## 7. Done criteria

- **Phase 1 done:** Tier-0 list (T0-1..T0-8) all closed; tests green;
  fixtures regenerated once.
- **Phase 2 done:** All non-[CONFIRMAR] Tier-1 and Tier-2 items closed;
  cross-doc checks gated behind repository ports; `IssueOptions`
  pattern adopted everywhere; `Cancel()` works with deadline.
- **Phase 3 ready:** seams exist for SAF-T, QR, PDF, AT webservices;
  module work can begin without re-opening `domain/` for plumbing.

---

## 8. Tracking

Use the IDs from this file (P1.N, P2.N, W-N) in commit messages and PR
titles. When closing an ID, link the GAP_ANALYSIS ID it resolves.

Example commit:
```
P1.3: enforce ATCUD code alphabet (closes T0-6)
```

---

## 8.5 AT-feedback open questions (added 2026-05-22)

The certification email (`AT_FEEDBACK.md` §8) raises questions that must
be answered before P2.17–P2.26 can land. Pull these into §0.5 once they
have answers.

1. **`SoftwareIdentity` storage** — build-time const, config file, or DB
   row? Must be identical across the certified release.
2. **Holiday calendar source** for F-NEW-5. Fallback: weekend-only.
3. **`DC` migration** — confirm no in-flight `DC` documents anywhere
   before P2.18 lands.
4. **`ProductDescription` policy** — A/B/C per `AT_FEEDBACK.md` §8 Q-4;
   default to B (snapshot + projector re-emit).
5. **Reprint authorization model** — out-of-domain port or in-domain
   permission tag? Drives F-NEW-12.
6. **`QRPayload` migration** — field reservation lands in P2.26; persisted
   docs must back-fill (or be re-issued — but issuance is immutable, so
   back-fill is the only path).
7. **Tier-3 QR module owner** — the field-by-field structure
   (`AT_FEEDBACK.md` §2) is settled, but generator selection (library
   vs hand-built) is open.

---

## 9. Open questions to flag before starting

1. **[CONFIRMAR] FS limits** — both retail and default need to be
   parameterized. Land P2.9 with `FSLimits{}` injected; fill values
   when legal team confirms. Fallback in §0.5.
2. **SAF-T (PT) v1.04_01 XSD obtained?** Drives both:
   - `MovementStatus T` semantics — verify before P2.5. Fallback in §0.5
     is to adopt the AT-public-docs reading (T = ThirdParty).
   - **Payment hash chain** (D-6) — drives whether cash-VAT signing
     stays deferred or moves into Phase 2.
3. **[CONFIRMAR] Cancellation deadline** — "day 5 of month+1, 23:59:59
   Europe/Lisbon" is asserted for e-Fatura submission; using the same
   rule for cancellation needs verification. P2.6 ships with this rule;
   adjust when confirmed.
4. **[CONFIRMAR] ATCUD code alphabet** — see §0.5 fallback.
5. **D-1 Money policy ratification** — Policy A vs Policy B. Without
   this decision, P1.6 can't start.
6. **Quantity precision** — fixing at 3 decimals (regras' optional
   restriction) vs SAF-T's 6. Recommendation: keep shared scale=100_000
   (i.e., 5 decimals internal, project to ≤3 at export) under Policy B;
   re-evaluate if fuel/utility customers ever need 4+.
7. **Money MarshalJSON shape** — emit `{cents: 4950}` or just `4950` as
   integer? Recommendation: integer cents; document on the type. DTOs
   above the domain layer can format for human display.
