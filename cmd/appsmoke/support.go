package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/flyzard/invoicing.v2/internal/app"
)

// Clock advances time minute-by-minute starting from a fixed base. Tick returns
// the next "now"; it is the Clock the service reads (Clock.Now()) when stamping
// SystemEntryDate and when checking cancellation deadlines.
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

// SetBase resets the clock's current time in place. appsmoke jumps from the
// previous month to the current one mid-run; mutating rather than replacing the
// clock keeps any holder of the pointer in sync.
func (c *Clock) SetBase(t time.Time) { c.current = t }

// Store collects every issued (and cancelled) document's IssuedView so the PDF
// pass can render each one through app.RenderPDF. Cancellation re-records the
// same number; the latest view wins.
type Store struct {
	byNumber map[string]app.IssuedView
	order    []string // first-seen order, for stable iteration
}

func NewStore() *Store {
	return &Store{byNumber: map[string]app.IssuedView{}}
}

func (s *Store) record(v app.IssuedView) {
	if _, seen := s.byNumber[v.Number]; !seen {
		s.order = append(s.order, v.Number)
	}
	s.byNumber[v.Number] = v
}

// views returns every recorded document, sorted by (Type, Seq) so the PDF pass
// is deterministic across runs.
func (s *Store) views() []app.IssuedView {
	out := make([]app.IssuedView, 0, len(s.order))
	for _, n := range s.order {
		out = append(out, s.byNumber[n])
	}
	slices.SortFunc(out, func(a, b app.IssuedView) int {
		if a.Type != b.Type {
			return strings.Compare(a.Type, b.Type)
		}
		return a.Seq - b.Seq
	})
	return out
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

func cents(c int64) string { return fmt.Sprintf("%d.%02d", c/100, c%100) }

// expectTotals is the smoke's one money check: it fails the run if a document's
// computed totals diverge from the reviewed certification dataset, comparing at
// cent precision. tax mirrors SAF-T TaxPayable (VAT + stamp).
func expectTotals(label string, net, tax, gross, wantNet, wantTax, wantGross int64) {
	if net != wantNet || tax != wantTax || gross != wantGross {
		log.Fatalf("ASSERT %s: got NET %s / TAX %s / GROSS %s, want %s / %s / %s",
			label, cents(net), cents(tax), cents(gross), cents(wantNet), cents(wantTax), cents(wantGross))
	}
	fmt.Printf("✓ %s totals match: NET %s · TAX %s · GROSS %s\n", label, cents(net), cents(tax), cents(gross))
}

// expectDoc asserts an IssuedView's totals against the reviewed cents.
func expectDoc(label string, v app.IssuedView, wantNet, wantTax, wantGross int64) {
	expectTotals(label, v.NetCents, v.TaxCents+v.StampCents, v.GrossCents, wantNet, wantTax, wantGross)
}

// checklist accumulates one row per issued document so appsmoke can emit the
// "ponto → documento → PDF" map the certification letter asks for (Nota 2).
var checklist []string

func recordChecklist(point string, v app.IssuedView) {
	checklist = append(checklist, fmt.Sprintf("%-28s %-22s %s", point, v.Number, pdfName(v, app.Original)))
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

func salesSummary(prefix string, v app.IssuedView) {
	recordChecklist(prefix, v)
	summary(fmt.Sprintf("%s · %s · ATCUD %s · NET %s · TAX %s · GROSS %s",
		prefix, v.Number, v.ATCUD,
		cents(v.NetCents), cents(v.TaxCents+v.StampCents), cents(v.GrossCents)))
}

func workSummary(prefix string, v app.IssuedView) {
	recordChecklist(prefix, v)
	summary(fmt.Sprintf("%s · %s · ATCUD %s · GROSS %s", prefix, v.Number, v.ATCUD, cents(v.GrossCents)))
}

func stockSummary(prefix string, v app.IssuedView) {
	recordChecklist(prefix, v)
	summary(fmt.Sprintf("%s · %s · ATCUD %s · GROSS %s", prefix, v.Number, v.ATCUD, cents(v.GrossCents)))
}

func paymentSummary(prefix string, v app.IssuedView) {
	recordChecklist(prefix, v)
	summary(fmt.Sprintf("%s · %s · ATCUD %s · GROSS %s", prefix, v.Number, v.ATCUD, cents(v.GrossCents)))
}

// printCancelledPDF renders a text-mode "PDF" with a prominent ANULADO banner —
// the inspector-facing stand-in so the cancellation is visibly marked.
func printCancelledPDF(v app.IssuedView) {
	const w = 60
	bar := strings.Repeat("─", w)
	fmt.Println()
	fmt.Println("┌" + bar + "┐")
	line := func(s string) { fmt.Printf("│ %-*s │\n", w-2, s) }
	line("*** DOCUMENTO ANULADO ***")
	line(strings.Repeat("─", w-2))
	line("")
	line(fmt.Sprintf("Documento: %s", v.Number))
	line(fmt.Sprintf("ATCUD:     %s", v.ATCUD))
	line(fmt.Sprintf("Data emissão: %s", v.Date))
	line(fmt.Sprintf("Anulado em:   %s", v.StatusDate))
	line(fmt.Sprintf("Motivo:    %s", v.Reason))
	line("")
	line(fmt.Sprintf("Cliente:   %s (NIF %s)", v.Customer.Name, v.Customer.TaxID))
	line("")
	line(fmt.Sprintf("NET:    %s €", cents(v.NetCents)))
	line(fmt.Sprintf("IVA:    %s €", cents(v.TaxCents)))
	line(fmt.Sprintf("TOTAL:  %s €", cents(v.GrossCents)))
	line("")
	line(fmt.Sprintf("Hash (pos 1,11,21,31): %c%c%c%c", v.Hash[0], v.Hash[10], v.Hash[20], v.Hash[30]))
	line("*** DOCUMENTO ANULADO — NÃO É VÁLIDO PARA EFEITOS FISCAIS ***")
	fmt.Println("└" + bar + "┘")
}

// printSAFTCancelRow shows the SAF-T DocumentStatus fields the projector emits
// for the cancelled document.
func printSAFTCancelRow(v app.IssuedView) {
	fmt.Println()
	fmt.Println("-- SAF-T SourceDocuments/SalesInvoices/Invoice/DocumentStatus --")
	fmt.Printf("  InvoiceStatus:     %s\n", v.Status)
	fmt.Printf("  InvoiceStatusDate: %s\n", v.StatusDate)
	fmt.Printf("  Reason:            %s\n", v.Reason)
	fmt.Printf("  SourceID:          %s\n", v.SourceID)
	fmt.Printf("  SourceBilling:     %s\n", v.SourceBilling)
}
