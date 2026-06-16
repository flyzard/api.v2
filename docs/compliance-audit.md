# AT Compliance Audit — Portuguese Certified Invoicing Engine

**Date:** 2026-06-15
**Method:** Multi-agent audit of 11 compliance regimes, each mapped then adversarially gap-checked against the source tree; high-impact wiring claims re-verified directly by grep + build + full test run.
**Scope:** `internal/domain`, `internal/adapter/*`, `internal/app`, `internal/config`, `cmd/*` (~19.7k LOC Go).

---

## What this engine must satisfy

| Regulation | Surface |
|---|---|
| **Portaria 363/2010** (Art. 5, Art. 6, Anexo II) | RSA-SHA1 producer signature, canonical string, per-series hash chain, print mention, recovery flows |
| **Despacho 8632/2014** §4.1/§4.2 | SHA-1 digest, RSA key exactly 1024 bits |
| **Portaria 195/2020** + Despacho 412/2020 | ATCUD, series lifecycle (SeriesWS), QR fields A–S, QR ≥30×30mm / v≥9 / ECC M |
| **SAF-T (PT) 1.04_01** (Portaria 302/2016, 31/2019) | XML export, Windows-1252, control sums, Hash placement |
| **DL 28/2019** | Real-time invoice comm (fatcorews), per-tenant election |
| **DL 147/2003** | Transport-doc comm (sgdtws) |
| **CIVA** Art. 36/40/78, **RITI** Art. 14 | Invoice content, FS ceilings, NC/ND rectification, M16 intra-EU |

## Bottom line

The regulatory **logic** is largely certification-grade. The blockers are in **wiring**, not in the law. The pure domain (signing, hash chain, ATCUD/QR, numbering, document rules, money) is carefully built and well-pinned by golden tests. But the application layer that would actually *run* it against AT in production does not exist yet, and one fiscal-integrity validator is built-but-disconnected. As shipped, the system **cannot emit a single genuinely certified document end-to-end.**

## Dimension scorecard

| # | Dimension | Core verdict | Top issue |
|---|---|---|---|
| 1 | Signature & hash chain | ✅ Certification-grade | Defense-in-depth only (no self-verify) |
| 2 | ATCUD & QR (invoices) | ✅ Certification-grade | ❓ Receipts carry **no QR at all** |
| 3 | SAF-T export | 🟡 Well-built projector | ⛔ **No XSD validation runs anywhere** |
| 4 | Series & numbering | ✅ Certification-grade | Cancel/finalize use stale snapshot |
| 5 | Document types & rules | ✅ Structurally sound | ⛔ NC/ND integrity (see #8) |
| 6 | AT webservices | 🟡 Protocol-faithful | ⛔ **No production client factory** |
| 7 | Recovery & provenance | 🟡 Domain correct | Unreachable via app layer |
| 8 | Allocations | ⛔ **Validator is dead code** | Over-settlement unenforced |
| 9 | PDF print | ✅ Certification-grade | No semantic QR≥30mm test |
| 10 | Money / tax / currency | 🟡 Strong core | SAF-T reconciliation drift |
| 11 | Software identity | ⛔ **No real signer wired** | config.Validate is dead |

---

## ⛔ Certification blockers (verified directly)

**B1 — No production composition root emits real signatures.** `NewRSASigner` is called only from tests. `AT_SIGNING_KEY_FILE` is read into `config.go:31` and **consumed by nothing**. The only runnable binary (`cmd/appsmoke`) signs with a non-RSA SHA-512 `stubSigner` (`main.go:35`). `cmd/demo` (referenced by CLAUDE.md) doesn't exist. → There is no path that produces a Portaria-363 RSA-SHA1 signature.

**B2 — No production `ATClientFactory`.** Zero non-test `ForTenant` implementations. The app layer literally cannot talk to AT (SeriesWS / fatcorews / sgdtws). Compounded: `ATPublicKey` is optional with no guard, so a future factory could silently send the WS-Security password **in plaintext** (`client.go:182-194`), and there is no `AT_ENABLED`/prod-env gate.

**B3 — `ValidateAllocations` is dead code.** The entire over-settlement / over-rectification protection exists, is tested across 9 branches, and has **zero callers** (confirmed by grep). Consequences as wired today:
- Two RC receipts can each fully settle the same invoice (no `Consumed` total is ever loaded).
- An NC can credit 10× the invoice it references (no monetary ceiling; NC doesn't even resolve the reference).
- NC/ND can reference a **cancelled** or **different-customer** invoice (`validateNDProductSet` checks only product/quantity).

CLAUDE.md:33 describes the validator as the live mechanism — doc-vs-reality mismatch.

**B4 — `config.Validate()` (the cert-metadata boot guard) is dead in every real entrypoint.** `config.Load` is called only by `cmd/atsmoke` and its result is **discarded**; `cmd/appsmoke` builds `SoftwareIdentity` by hand and never validates.

**B5 — No XSD validation in the SAF-T export path.** `Export()` only marshals + transcodes. The bundled schema is XSD 1.1 (`xs:assert`, unbounded `maxOccurs`) so `xmllint` can't compile it; the only validation test is `t.Skip()`ped unless an external Saxon/Xerces is wired. → The golden file guards against *drift*, not against *schema validity*. This converts every "projector trusts upstream" gap below into a silently-shippable risk.

**B6 — `CertificateNumber` not constrained to AT's numeric type.** SAF-T XSD types `SoftwareCertificateNumber` as `xs:nonNegativeInteger`; config only length-checks it. A non-numeric value flows verbatim into Header + QR field R + PDF mention. With B4 + B5, nothing catches it.

---

## 🟡 Correctness gaps (MEDIUM — would cause real AT rejections)

**SAF-T money reconciliation:**
- **Line drift:** projector emits `UnitPrice=EffectiveUnitPrice` (5dp) and an independently-computed `CreditAmount`. AT recomputes `UnitPrice × Quantity`; for large fractional quantities they diverge — fuzz found a **5-cent** gap, beyond AT's ~0.01 line tolerance.
- **`TaxBase` silently dropped:** domain + PDF support tax-only adjustment lines (`TaxBase`, `UnitPrice=0`), but the SAF-T line projector has no `TaxBase` element — exports tax with no base, unreconcilable.
- **`Currency.Amount` not bound to `GrossTotal`:** `TestExport_CurrencyDirection` exports foreign **net** (347.20) as the currency amount for a 393.60 gross — and passes.
- **Totals identity:** `round(Net)+round(Tax)` can differ from `round(Gross)` by a cent (reproduced) — independent 2dp rounding of sub-cent accumulators.

**AT communication completeness:**
- **Transport (sgdtws) unwired** — GT/GR/GA/GC/GD issued, signed, hash-chained, never communicated. DL 147/2003 obligation unmet. `commsRequired` discards its `DocumentType` arg.
- **Cancellations never sent** — `CancelDocument` only mutates local state; a CommRealtime tenant's cancelled invoice stays "live" in e-Fatura until next SAF-T.
- **`CashVATSchemeIndicator` hardcoded 0** in fatcorews while SAF-T reads `inv.SpecialRegimes.CashVAT` → cross-channel divergence (one-line fix).
- **No lost-response reconciliation** for fatcorews/sgdtws (only SeriesWS self-reconciles); no client idempotency token.

**Recovery:** logic correct in the domain but **unreachable through `InvoicingService`** — no `IntegrateRecovered*` entry point.

**Series:** `CancelSeries`/`FinalizeSeries` build the AT request from a **stale, lock-free snapshot** → could send a false "no documents issued" declaration for a now-used series.

---

## ❓ Open legal questions — need AT spec confirmation, not code

1. **Receipts (RC/RG) carry NO QR code at all** (deliberate, justified by "no Hash"). If AT requires a receipt QR, every issued RC/RG is non-compliant. Highest-consequence open item.
2. **NC monetary ceiling** — code self-flags `[CONFIRMAR] — no CIVA article caps NC amounts`.
3. **Recovery D-form** — `OriginalType` not constrained to equal recovered type (`[CONFIRMAR]`); `meioProcessamento` hardcoded `"PI"` for tipoSerie-`R` series, never exercised live.
4. **`ThirdParties` print mention** — modelled, never printed.
5. **Field C `"Desconhecido"`** and **TaxTable completeness for IS/NS** lines.

---

## ✅ What's genuinely solid (don't touch without regulatory review)

- **Hash chain & signing:** canonical string byte-pinned (`InvoiceDate;SystemEntryDateTime;DocNo;Gross(2dp);PrevHash`), RSA-SHA1 gated to exactly 1024 bits, single chain-mutation point, genesis empty-prev handled, receipts correctly excluded. Signed `GrossTotal` and exported `GrossTotal` share identical `Format2DP` rendering — AT can independently re-verify every signature.
- **ATCUD/QR (invoice family):** fields A–S in order, I/J/K region grammar, field Q from positions 1/11/21/31, **version≥9 + ECC M enforced unconditionally** with gozxing round-trip proof.
- **Series & gapless numbering:** single `LastNum+1` source, per-series mutex + optimistic version + retry, proven 1..N under 25-goroutine concurrency; never-issued→cancel / used→finalize guarded at both layers.
- **SAF-T structure:** element ordering hand-verified against the XSD, Windows-1252 single-gate transcode, per-family control-sum exclusion rules correct, Hash present on sales/work/movement and correctly absent on Payments.
- **WS-Security crypto:** AES-128-ECB password under RSA-encrypted nonce, fresh credentials per retry — faithful to AT's spec.
- **PDF print:** cert mention, 4 signature chars, ATCUD-above-QR (repeated per page), QR at 32mm, frozen exemption motives, FS-only-NIF / "Consumidor final" suppression.

---

## Recommended sequence to certification-readiness

1. **Wire a production composition root** (B1+B2+B4+B6): real RSA signer + key version from config, production `ATClientFactory` with plaintext refusal, invoke `config.Validate` + constrain `CertificateNumber` numeric.
2. **Connect `ValidateAllocations`** into IssuePayment and the NC/ND path (B3) + add per-source-document locking against concurrent settlement.
3. **Fix SAF-T money reconciliation** (line drift, `TaxBase` element, `Currency.Amount==GrossTotal` guard, totals identity assertion).
4. **Wire a real XSD 1.1 validator into CI** (B5) over a fixture matrix (NC, exempt, NS, stamp, RC, FX).
5. **Resolve the open legal questions** with AT — starting with the **receipt-QR** question.
6. **Implement transport comm + cancellation comm**; feed CashVAT into fatcorews.

---

## Notes

- Build and full test suite are **green** (`go build ./...`, `go test ./...` all pass).
- Doc drift: `cmd/demo` and `docs/superpowers/specs|plans` referenced in CLAUDE.md don't exist (now `cmd/appsmoke`); recovery design spec referenced in `recovery.md` is missing.
