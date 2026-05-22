package main

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/flyzard/invoicing.v2/domain"
)

// stubSigner produces a deterministic 172-char base64 hash from prevHash and
// the canonical signing line. NOT real RSA-SHA1 — this is dev plumbing so the
// demo can advance the hash chain without an AT-issued private key.
type stubSigner struct{}

func (stubSigner) Sign(prevHash, canonical string) (string, string, error) {
	payload := []byte(prevHash + "|" + canonical)
	a := sha512.Sum512(payload)
	b := sha512.Sum512(a[:])
	full := append(a[:], b[:]...) // 128 bytes → base64 is exactly 172 chars
	return base64.StdEncoding.EncodeToString(full), "1", nil
}

// monotonicClock advances time minute-by-minute starting from a fixed base.
// Tick returns the next "now" and is also the Clock that IssuedDocument.Cancel
// reads when checking the e-Fatura deadline.
type monotonicClock struct {
	current time.Time
	step    time.Duration
}

func newClock(base time.Time, step time.Duration) *monotonicClock {
	return &monotonicClock{current: base, step: step}
}

func (c *monotonicClock) Now() time.Time { return c.current }

func (c *monotonicClock) Tick() time.Time {
	c.current = c.current.Add(c.step)
	return c.current
}

// memoryStore is the in-memory IssuedDocumentReader used by ND issuance to
// resolve References against originating invoices. Every issued sales document
// is recorded so later scenarios can reference earlier ones.
type memoryStore struct {
	docs map[string]domain.IssuedDocument
}

func newStore() *memoryStore {
	return &memoryStore{docs: map[string]domain.IssuedDocument{}}
}

func (s *memoryStore) record(d domain.IssuedDocument) {
	s.docs[d.Number.Format()] = d
}

func (s *memoryStore) FindByNumber(n domain.DocNumber) (domain.IssuedDocument, error) {
	d, ok := s.docs[n.Format()]
	if !ok {
		return domain.IssuedDocument{}, fmt.Errorf("not found: %s", n.Format())
	}
	return d, nil
}

func must[T any](v T, err error) T {
	if err != nil {
		log.Fatalf("setup: %v", err)
	}
	return v
}

func banner(num, title string) {
	bar := strings.Repeat("=", 78)
	fmt.Println()
	fmt.Println(bar)
	fmt.Printf("=== %s — %s\n", num, title)
	fmt.Println(bar)
}

func printJSON(label string, v any) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatalf("marshal %s: %v", label, err)
	}
	if label != "" {
		fmt.Printf("-- %s --\n", label)
	}
	fmt.Println(string(out))
}

func summary(line string) {
	fmt.Printf("→ %s\n", line)
}

func salesSummary(prefix string, doc domain.SalesInvoice) {
	summary(fmt.Sprintf("%s · %s · ATCUD %s · NET %s · TAX %s · GROSS %s",
		prefix,
		doc.Number.Format(),
		doc.ATCUD,
		doc.Totals.NetTotal.Format2DP(),
		(doc.Totals.TaxTotal + doc.Totals.StampDuty).Format2DP(),
		doc.Totals.GrossTotal.Format2DP(),
	))
}

func workSummary(prefix string, doc domain.WorkDocument) {
	summary(fmt.Sprintf("%s · %s · ATCUD %s · GROSS %s",
		prefix, doc.Number.Format(), doc.ATCUD, doc.Totals.GrossTotal.Format2DP()))
}

func stockSummary(prefix string, doc domain.StockMovement) {
	summary(fmt.Sprintf("%s · %s · ATCUD %s · GROSS %s",
		prefix, doc.Number.Format(), doc.ATCUD, doc.Totals.GrossTotal.Format2DP()))
}

func paymentSummary(prefix string, doc domain.Payment) {
	summary(fmt.Sprintf("%s · %s · ATCUD %s · GROSS %s",
		prefix, doc.Number.Format(), doc.ATCUD, doc.PaymentTotals.GrossTotal.Format2DP()))
}

// printCancelledPDF renders a text-mode "PDF" with a prominent ANULADO banner.
// Real PDF rendering lives in a Tier-3 module; this is the inspector-facing
// stand-in so the cancellation is visibly marked.
func printCancelledPDF(doc domain.SalesInvoice) {
	const w = 60
	bar := strings.Repeat("─", w)
	fmt.Println()
	fmt.Println("┌" + bar + "┐")
	line := func(s string) { fmt.Printf("│ %-*s │\n", w-2, s) }
	line("*** DOCUMENTO ANULADO ***")
	line(strings.Repeat("─", w-2))
	line("")
	line(fmt.Sprintf("Documento: %s", doc.Number.Format()))
	line(fmt.Sprintf("ATCUD:     %s", doc.ATCUD))
	line(fmt.Sprintf("Data emissão: %s", doc.Date.Format("2006-01-02")))
	line(fmt.Sprintf("Anulado em:   %s", doc.StatusDate.Format("2006-01-02 15:04")))
	line(fmt.Sprintf("Motivo:    %s", doc.Reason))
	line("")
	line("Emitente:  Demo Faturação Lda. (NIF 500000000)")
	line(fmt.Sprintf("Cliente:   %s (NIF %s)", doc.Customer.CompanyName, doc.Customer.CustomerTaxID))
	line("")
	line(fmt.Sprintf("NET:    %s €", doc.Totals.NetTotal.Format2DP()))
	line(fmt.Sprintf("IVA:    %s €", doc.Totals.TaxTotal.Format2DP()))
	line(fmt.Sprintf("TOTAL:  %s €", doc.Totals.GrossTotal.Format2DP()))
	line("")
	line(fmt.Sprintf("Hash (pos 1,11,21,31): %c%c%c%c", doc.Hash[0], doc.Hash[10], doc.Hash[20], doc.Hash[30]))
	line("*** DOCUMENTO ANULADO — NÃO É VÁLIDO PARA EFEITOS FISCAIS ***")
	fmt.Println("└" + bar + "┘")
}

// printSAFTCancelRow shows the SAF-T DocumentStatus fields that the projector
// (Tier-3 module) will emit for the cancelled document. Inspector reads this to
// confirm the cancellation registered in both the DB (issued doc state) and
// the SAF-T export shape.
func printSAFTCancelRow(doc domain.SalesInvoice) {
	fmt.Println()
	fmt.Println("-- SAF-T SourceDocuments/SalesInvoices/Invoice/DocumentStatus --")
	fmt.Printf("  InvoiceStatus:     %s\n", doc.Status)
	fmt.Printf("  InvoiceStatusDate: %s\n", doc.StatusDate.Format("2006-01-02T15:04:05"))
	fmt.Printf("  Reason:            %s\n", doc.Reason)
	fmt.Printf("  SourceID:          %s\n", doc.SourceID)
	fmt.Printf("  SourceBilling:     %s\n", doc.SourceBilling)
}

// printSettlementSimulation prints a doc-level Settlement block. Domain does
// not yet store doc-level discounts on SalesInvoice; the SAF-T projector
// (Tier-3 module) will source this from a dedicated field once added. For now
// the inspector sees it stitched on alongside the issued JSON.
func printSettlementSimulation(amount domain.Money, reason string) {
	fmt.Println()
	fmt.Println("-- SAF-T DocumentTotals/Settlement (simulated — not yet persisted) --")
	fmt.Printf("  SettlementAmount:      %s\n", amount.Format2DP())
	fmt.Printf("  SettlementDescription: %s\n", reason)
}

// printCurrencySimulation prints a foreign-currency block. Domain attaches
// Currency only to Payments today; for sales invoices it is simulated here so
// the inspector can see the projected SAF-T DocumentTotals/Currency shape.
func printCurrencySimulation(c domain.Currency) {
	fmt.Println()
	fmt.Println("-- SAF-T DocumentTotals/Currency (simulated — not yet persisted on SalesInvoice) --")
	fmt.Printf("  CurrencyCode:   %s\n", c.Code)
	fmt.Printf("  CurrencyAmount: %s\n", c.Amount.Format2DP())
	fmt.Printf("  ExchangeRate:   %.6f\n", c.ExchangeRate.Float64())
	fmt.Printf("  Date:           %s\n", c.Date.Format("2006-01-02"))
}
