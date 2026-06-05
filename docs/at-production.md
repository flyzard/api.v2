# Switching the AT webservices to production

How to move the `internal/adapter/at` client from the AT test environment (everything
live-verified there on 2026-06-05) to production. Sources: AT manuals
"Comunicação de Séries Documentais — Aspetos Específicos" v1.2, "e-Fatura —
Comunicação por webservice, Aspetos genéricos" §2.1.3–2.1.4, plus what we
learned the hard way (see Gotchas).

## 1. Test vs production matrix

| Item | Test (current) | Production |
|---|---|---|
| SeriesWS URL | `at.TestSeriesURL` (`:722/SeriesWSService`) | `at.ProductionSeriesURL` (`:422/SeriesWSService`) |
| Transport URL | `at.TestTransportURL` (`:701/sgdtws/documentosTransporte`) | `at.ProductionTransportURL` (`:401/...`) |
| Invoice URL | `at.TestInvoiceURL` (`:723/fatcorews/ws/`) | `at.ProductionInvoiceURL` (`:423/fatcorews/ws/`) |
| Client TLS cert | `TesteWebservices` pair (`api/certs/at-test.crt|.key`, **expires 2026-07-18**) | Producer-specific cert: CSR signed by AT (see §3) |
| AT cipher public key | `api/certs/at-public-key.pem` ("Chave Cifra Publica AT", 4096-bit) | Same key — environment-independent |
| Credentials | Taxpayer sub-user `NIF/n` with WSE+WDT+WFA | Same model; **each taxpayer** creates their own sub-user with the same operations |
| `AT_CERT_NUM` (numCertSWFatur) | `0` (uncertified) | Real AT software-certificate number |
| Document signer | `stubSigner` (smoke only) | `signing.NewRSASigner` with the AT-registered private key + key version |
| Series / ATCUD codes | Test codes (`AAJF...`) — **invalid in production** | Re-register every series in production; production `codValidacaoSerie` |

All three URLs are plain constants on `at.Config` — production switch-over is
configuration, not code.

## 2. Legal / process prerequisites (before touching production)

1. **Software certification (Portaria 363/2010)** — obtain the AT software
   certificate number. That number becomes `AT_CERT_NUM` (`numCertSWFatur` in
   registarSerie, `SoftwareCertificateNumber` in fatcorews). The RSA signing
   key pair used by `AT_SIGNING_KEY_FILE` is registered with AT during
   certification, with its key version. Once `AT_CERT_NUM != 0`, fatcorews
   `HashCharacters` automatically switches from `"0"` to the real hash chars
   (positions 1/11/21/31) — no code change needed.
2. **e-Fatura service adhesion (software producer)** — Portal das Finanças →
   e-Fatura → Produtores de Software → *Aderir ao Serviço*. Accept the terms,
   generate a **CSR** per manual §2.3.1, submit it; AT e-mails back the
   **production SSL certificate** signed by AT. This replaces the
   TesteWebservices pair for production connections (Aspetos genéricos §2.1.3).
3. **Per-taxpayer sub-user** — each cliente (sujeito passivo) creates a
   sub-user at Portal das Finanças → Gestão de Utilizadores with operations:
   - **WSE** — Comunicação e Gestão de Séries por webservice
   - **WDT** — Webservice de comunicação de documentos de transporte
   - **WFA** — Webservice de comunicação de dados de faturas
   The program stores those credentials (our `AT_NIF` / `AT_USERNAME` /
   `AT_PASSWORD` model — username accepts both `n` and `NIF/n` forms).
4. **Production series registration** — every series must be registered via
   registarSerie against the **production** SeriesWS before issuing. Test-env
   validation codes/ATCUDs are meaningless in production.

## 3. Configuration changes

In whatever wires `at.NewClient` (today only `cmd/atsmoke`; the M1 app layer
later):

```go
client, err := at.NewClient(at.Config{
    SeriesURL:       at.ProductionSeriesURL,
    TransportURL:    at.ProductionTransportURL,
    InvoiceURL:      at.ProductionInvoiceURL,
    TaxpayerNIF:     cfg.ATNIF,
    Username:        cfg.ATUsername,
    Password:        cfg.ATPassword,
    SoftwareCertNum: cfg.ATCertNum,        // real cert number, not "0"
    ATPublicKey:     atPub,                // same Chave Cifra Publica AT
    Certificate:     prodCert,             // AT-signed production cert (CSR flow)
    // Defaults already production-appropriate: rate 5 req/s burst 10,
    // 3-attempt retry with backoff, 30s timeouts.
})
```

Env-file equivalent (the v1 app gated this with `AT_USE_TEST_ENV=false` +
`AT_ENABLED=true`; the M1 app layer should reintroduce that pattern so
production is an explicit opt-in, never a default):

```bash
AT_PUBLIC_KEY_FILE=/path/to/at-public-key.pem      # unchanged
AT_CLIENT_CERT_FILE=/path/to/production.crt       # AT-signed, from adhesion
AT_CLIENT_KEY_FILE=/path/to/production.key
AT_CERT_NUM=<real certificate number>
AT_NIF=<taxpayer NIF>
AT_USERNAME=<taxpayer sub-user>
AT_PASSWORD=<sub-user password>
AT_SIGNING_KEY_FILE=/path/to/registered-signing-key.pem
```

**Do NOT** point `cmd/atsmoke` at production. It registers, cancels and
finalizes throwaway series — operations with legal meaning in production. The
smoke hardcodes the test URLs deliberately; keep it that way.

## 4. First-run verification order (safe ramp)

Production has no sandbox semantics — every call is a legal communication.
Ramp in this order, verifying each step before the next:

1. **consultarSeries** on a known/nonexistent series — proves auth + TLS +
   envelope against production without side effects (worst case: "no results").
2. **registarSerie** for the first real series → store the returned
   `codValidacaoSerie` → `Series.RegisterWithAT`. Verify with consultarSeries.
3. **Issue one real document** in that series; check ATCUD/QR on the output.
4. **fatcorews RegisterInvoice** for that document → expect `code=0`.
   (Real-time webservice is one allowed channel under DL 28/2019; until this
   step is trusted, the monthly SAF-T submission still satisfies the duty.)
5. **sgdtws** only when a real goods movement exists — the returned
   `ATDocCodeID` must be printed on the transport document **before goods
   move**. Never fire test transports in production.
6. Series **finalizarSerie/anularSerie** only as genuine lifecycle events
   (anular only for registered-in-error, never-used series; motivo `ER`).

## 5. Gotchas (learned live, 2026-06-05)

- **Never trust v1's endpoint table.** v1 carried fatcorews `:700`/`:400` —
  wrong; the manual says `:723`/`:423`. `:700` answers *every* request with a
  generic `env:Client` 500 "Internal Error". If you see that fault, check the
  endpoint before debugging the envelope.
- **`codValidacaoSerie` is minLength 8, no maximum** (live code was 10 chars).
  Anything persisting it must not assume fixed length.
- **The TesteWebservices cert is test-only and expires 2026-07-18.** Renew it
  for continued test-env work; it has no role in production.
- **Two-phase ordering is the caller's job**: call the webservice first, mutate
  the domain `Series` only after AT accepts (`RegisterWithAT`/`Finalize`/
  `Cancel`). The adapter never touches domain state.
- **Transport docs**: AT validates customer NIF checksums server-side (code
  `-1`); cancellation submissions return no new `ATDocCodeID` (handled).
- **Recovery series (`tipoSerie R`)** is sourced from the manual (§1.3.6) but
  has not been exercised live even in test — prove it in test before the first
  production recovery integration.

## 6. Out of scope until M1 (app layer)

- `AT_ENABLED` / `AT_USE_TEST_ENV` config gates (v1 pattern: production
  validation forbade NullClient and test endpoints).
- Communication status persistence + retry queue (v1 `ATCommunicationInfo`).
- fatcorews `ChangeInvoiceStatus` / `DeleteInvoice` and the fatshare query
  service (`:425/fatshare/ws/fatshareFaturas` test `:725`) — not implemented
  (v1 didn't have them either).
