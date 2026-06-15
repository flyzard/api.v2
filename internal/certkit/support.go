package certkit

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/flyzard/invoicing.v2/internal/adapter/pdf"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// Clock advances time minute-by-minute starting from a fixed base.
// Tick returns the next "now" and is also the Clock that IssuedDocument.Cancel
// reads when checking the e-Fatura deadline.
type Clock struct {
	current time.Time
	step    time.Duration
}

func NewClock(base time.Time, step time.Duration) *Clock {
	return &Clock{current: base, step: step}
}

func (c *Clock) Now() time.Time { return c.current }

func (c *Clock) Tick() time.Time {
	c.current = c.current.Add(c.step)
	return c.current
}

// SetBase resets the clock's current time in place. The demo jumps from the
// previous month to the current one mid-run; mutating rather than replacing the
// clock keeps any holder of the pointer (e.g. a DomainIssuer) in sync.
func (c *Clock) SetBase(t time.Time) { c.current = t }

// Store holds issued documents per family so the projector and ND validation
// can consume them. Sales drives FindByNumber; the other families are kept for
// the SAF-T export pass.
type Store struct {
	sales    map[string]domain.SalesInvoice
	stock    map[string]domain.StockMovement
	work     map[string]domain.WorkDocument
	payments map[string]domain.Payment
}

func NewStore() *Store {
	return &Store{
		sales:    map[string]domain.SalesInvoice{},
		stock:    map[string]domain.StockMovement{},
		work:     map[string]domain.WorkDocument{},
		payments: map[string]domain.Payment{},
	}
}

func (s *Store) recordSales(d domain.SalesInvoice)  { s.sales[d.Number.Format()] = d }
func (s *Store) recordStock(d domain.StockMovement) { s.stock[d.Number.Format()] = d }
func (s *Store) recordWork(d domain.WorkDocument)   { s.work[d.Number.Format()] = d }
func (s *Store) recordPayment(d domain.Payment)     { s.payments[d.Number.Format()] = d }

// snapshot* return all recorded values as slices. Order is not guaranteed;
// the projector sorts deterministically per family at export time.
func (s *Store) snapshotSales() []domain.SalesInvoice {
	return slices.Collect(maps.Values(s.sales))
}
func (s *Store) snapshotStock() []domain.StockMovement {
	return slices.Collect(maps.Values(s.stock))
}
func (s *Store) snapshotWork() []domain.WorkDocument {
	return slices.Collect(maps.Values(s.work))
}
func (s *Store) snapshotPayments() []domain.Payment {
	return slices.Collect(maps.Values(s.payments))
}

func (s *Store) FindByNumber(n domain.DocNumber) (domain.IssuedDocument, error) {
	d, ok := s.sales[n.Format()]
	if !ok {
		return domain.IssuedDocument{}, fmt.Errorf("not found: %s", n.Format())
	}
	return d.IssuedDocument, nil
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

// checklist accumulates one row per issued document so the demo can emit the
// "ponto → documento → PDF" map the certification letter asks for (Nota 2).
var checklist []string

func recordChecklist(point string, n domain.DocNumber) {
	checklist = append(checklist, fmt.Sprintf("%-28s %-22s %s", point, n.Format(), pdfName(n, pdf.Original)))
}

// WriteChecklist writes <outDir>/CHECKLIST.txt mapping every checklist point to
// the issued document and the PDF file rendered for it.
func WriteChecklist(outDir string) {
	body := "AT certification §5 — ponto → documento → PDF\n\n" + strings.Join(checklist, "\n") + "\n"
	path := filepath.Join(outDir, "CHECKLIST.txt")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
	fmt.Printf("Checklist written: %s (%d rows)\n", path, len(checklist))
}

func salesSummary(prefix string, doc domain.SalesInvoice) {
	recordChecklist(prefix, doc.Number)
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
	recordChecklist(prefix, doc.Number)
	summary(fmt.Sprintf("%s · %s · ATCUD %s · GROSS %s",
		prefix, doc.Number.Format(), doc.ATCUD, doc.Totals.GrossTotal.Format2DP()))
}

func stockSummary(prefix string, doc domain.StockMovement) {
	recordChecklist(prefix, doc.Number)
	summary(fmt.Sprintf("%s · %s · ATCUD %s · GROSS %s",
		prefix, doc.Number.Format(), doc.ATCUD, doc.Totals.GrossTotal.Format2DP()))
}

func paymentSummary(prefix string, doc domain.Payment) {
	recordChecklist(prefix, doc.Number)
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
