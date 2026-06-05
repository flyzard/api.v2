# Document Recovery

## What it is

When the certified invoicing system is unavailable, Portuguese law (Portaria
363/2010) still lets a business keep selling — but every document issued outside
the certified path must later be **integrated back** into the certified system.
That integration is "document recovery". There are two flavours:

| Kind | Constant | Scenario |
|------|----------|----------|
| Manual (`'M'`) | `RecoveryManual` | Document written on pre-printed paper (tipografia autorizada) during an outage |
| Backup (`'D'`) | `RecoveryBackup` | Document issued by a backup certified system ("programa de recurso", Portaria 363/2010 Anexo II) |

The recovered document is re-issued **inside** the certified system: it gets a
new number in a recovery series, is signed into that series' hash chain, and
carries provenance markers pointing back at the original:

- `SourceBilling = "M"` (SAF-T: "documento recuperado / emitido manualmente")
- A composed `HashControl` that embeds the original document's identity:
  - M-form: `{key}-{type}M {série}/{número}` → e.g. `1-FTM F/23`
    (recovered FT, original paper doc was número 23 of série F)
  - D-form: `{key}-{type}D {origType} {série}/{número}` → e.g. `1-FTD FT D/3`
    (recovered FT, original was FT número 3 of série D in the backup system)

Receipts (RC/RG) carry no Hash/HashControl in SAF-T, so their recovery is just
`SourcePayment = "M"` plus the recovery-series policy — no original reference.

## Rules the domain enforces

1. **Dedicated recovery series.** Recovery only issues into a series created
   with `NewRecoverySeries` (its `ProcessingMeans` is `"A"`). Normal issuance
   into such a series is rejected (`ErrRecoverySeriesMisuse`); recovery into a
   normal series is rejected (`ErrNotRecoverySeries`). This keeps production
   series free of out-of-order dates.
2. **Invariant.** `IssueOptions.Recovered != nil ⟺ SourceBilling == "M"`.
   The `IntegrateRecovered*` functions set both for you; calling the plain
   `Issue*` functions with only one of them errors.
3. **Guards bypassed.** Recovery skips the monotonic-date-in-series guard, the
   CIVA Art. 36.º 5-working-day emission cap, and (for guias) the
   start-before-system-entry check — recovered originals necessarily predate
   their integration.
4. **Guards NOT bypassed.** Everything else still applies: draft validation,
   FS gross-total cap (pass `IssuerEAC`/`FSLimits` via opts), ND product-set
   check (pass `Reader` via opts), FR payment-sum check, system entry date
   cannot precede the document date, and the M16 gate (RITI Art. 14.º: an
   M16-exempt line requires an EU non-PT customer with a VAT id). The original
   was only legal if those conditions held when it was issued, so the
   reconstructed draft must carry the complete customer data — deliberate
   strictness, not an oversight.
5. **Hash chain.** The recovered document signs over the recovery series' own
   chain with its new number. The original's identity lives only in the
   HashControl — there is no separate stored field.

## How to use

### 1. Create and register a recovery series (once)

```go
series, err := domain.NewRecoverySeries("REC2026", domain.FT)
if err != nil { ... }
// Communicate the série to AT as usual, then:
err = series.RegisterWithAT(atValidationCode, time.Now())
```

A recovery series is per document type, like any other series. It must be
registered with AT before it can issue (`CanIssue` applies unchanged).

### 2. Integrate a manual (paper) document

```go
draft := &domain.DraftSalesInvoice{ /* re-key the paper document's data */ }
// draft.Date = the date PRINTED on the paper original.

ref := domain.RecoveredRef{
    Kind:           domain.RecoveryManual,
    OriginalSeries: "F",  // série printed on the paper doc
    OriginalNumber: 23,   // número printed on the paper doc
}

inv, err := domain.IntegrateRecoveredSalesInvoice(
    draft, ref, series, signer, sourceID, time.Now(), domain.IssueOptions{}, qrCfg)
// inv.HashControl == "1-FTM F/23", inv.SourceBilling == "M"
```

### 3. Integrate a backup-system document

Same call, different ref — `OriginalType` is required and names the document
type in the backup system:

```go
ref := domain.RecoveredRef{
    Kind:           domain.RecoveryBackup,
    OriginalType:   domain.FT,
    OriginalSeries: "D",
    OriginalNumber: 3,
}
// → HashControl "1-FTD FT D/3"
```

### 4. Other families

```go
// Transport documents (start-time check bypassed automatically):
domain.IntegrateRecoveredStockMovement(draft, ref, series, signer, sourceID, now, opts, qrCfg)

// Working documents:
domain.IntegrateRecoveredWorkDocument(draft, ref, series, signer, sourceID, now, opts, qrCfg)

// Receipts — no ref, no opts (nothing to encode / nothing applies):
domain.IntegrateRecoveredPayment(draft, series, now, totals)
```

### Passing options

The signed-family wrappers take your `IssueOptions` and force
`SourceBilling`/`Recovered` on top. Set the rest as you would for normal
issuance — e.g. a recovered FS needs `opts.IssuerEAC` for the retail-tier cap,
a recovered ND needs `opts.Reader`.

## Errors you may see

| Error | Cause |
|-------|-------|
| `ErrNotRecoverySeries` | Recovery aimed at a series not created via `NewRecoverySeries` |
| `ErrRecoverySeriesMisuse` | Normal issuance aimed at a recovery series |
| `recovered ref: …` | `RecoveredRef` invalid — bad kind, série with `/`, `^` or space, número < 1, `OriginalType` missing (backup) or present (manual) |
| `SourceBilling "M" requires IssueOptions.Recovered …` | Called a plain `Issue*` with `SourceBilling: "M"` but no ref — use the `IntegrateRecovered*` wrapper |
| `IssueOptions.Recovered is not applicable to payments …` | Receipts can't carry a ref; use `IntegrateRecoveredPayment` |

## Scope notes

- Printing the duplicate marking on recovered documents ("Cópia do documento
  original — …", Despacho 8632/2014) is a print-layer concern; the data needed
  for it is recoverable from the HashControl.
- Two points are marked **[CONFIRMAR]** in the code pending legal confirmation:
  the meaning of the D-form's first token (assumed: original document type) and
  whether `OriginalType` must equal the new document's type (assumed: no).
- Recovery of *cancellations* past the e-Fatura deadline is a different flow,
  not covered here.

Design rationale: `docs/superpowers/specs/2026-06-04-document-recovery-design.md`.
