// Command main runs the 13 scenarios required by the AT certification
// checklist (§5.1–5.13). Each scenario issues a real document through the
// domain layer, prints the issued JSON and a one-line summary, and — where
// applicable — simulates downstream artefacts (PDF, SAF-T rows) that live in
// Tier-3 modules not yet implemented.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/flyzard/invoicing.v2/internal/adapter/saft"
	"github.com/flyzard/invoicing.v2/internal/adapter/signing"
	"github.com/flyzard/invoicing.v2/internal/config"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

func main() {
	loc := mustLisbon()
	today := time.Date(2026, 5, 22, 0, 0, 0, 0, loc)
	clockBase := time.Date(2026, 5, 22, 9, 0, 0, 0, loc)
	// Previous-month prologue: the certification letter asks for documents
	// from two different months (campos 4.1.4.5 / 4.2.3.5 / 4.3.4.5), so a
	// few documents are issued in April before the May walkthrough.
	prevMonthDay := time.Date(2026, 4, 22, 0, 0, 0, 0, loc)
	prevMonthClock := time.Date(2026, 4, 22, 9, 0, 0, 0, loc)

	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatal(err)
	}

	if cfg.SigningKeyFile == "" {
		log.Fatal("AT_SIGNING_KEY_FILE is required")
	}
	pemBytes, err := os.ReadFile(cfg.SigningKeyFile)
	if err != nil {
		log.Fatalf("read signing key %s: %v", cfg.SigningKeyFile, err)
	}
	signer, err := signing.NewRSASigner(pemBytes, 1)
	if err != nil {
		log.Fatalf("signing key %s: %v", cfg.SigningKeyFile, err)
	}
	signerName := "RSA-SHA1 (Portaria 363/2010) · key version 1"

	// Fixtures (series registration) and the clock start in April so the
	// prologue documents carry April SystemEntryDates; per-series hash-chain
	// monotonicity holds because every May document is issued afterwards.
	f := buildFixtures(prevMonthClock)
	c := &ctx{
		f:      f,
		signer: signer,
		clock:  newClock(prevMonthClock, time.Minute),
		store:  newStore(),
		qr: domain.QRConfig{
			IssuerNIF:         f.Issuer.NIF,
			CertificateNumber: cfg.Software.CertificateNumber,
		},
	}

	fmt.Println("AT Certification Checklist — §5 walkthrough")
	fmt.Printf("Issuer: %s (NIF %s · EAC %s)\n", f.Issuer.Name, f.Issuer.NIF, f.Issuer.EACCode)
	fmt.Printf("Software: %s %s · cert %s\n", cfg.Software.ProductID(), cfg.Software.Version, cfg.Software.CertificateNumber)
	fmt.Printf("Document dates: %s (mês anterior) · %s (§5) · clock starts at %s\n",
		prevMonthDay.Format("2006-01-02"), today.Format("2006-01-02"), prevMonthClock.Format("2006-01-02T15:04 MST"))
	fmt.Printf("Signer: %s\n", signerName)

	scenarioPrevMonth(c, prevMonthDay)
	c.clock = newClock(clockBase, time.Minute) // jump from April to May

	scenario51(c, today)
	scenario52(c, today)
	scenario53(c, today)
	scenario54(c, today)
	scenario55(c, today)
	scenario56(c, today)
	scenario57(c, today)
	scenario58(c, today)
	scenario59(c, today)
	scenario510(c, today)
	scenario511(c, today)
	scenario512(c, today)
	scenario513(c, today)

	writeSAFT(c, f, cfg.Software, today)
	writeDocumentPDFs(c, f, cfg.Software)
	writeChecklist()

	fmt.Println()
	fmt.Println("Done.")
}

// writeSAFT projects every recorded document into a single SAF-T XML file
// under out/. The period spans the previous month through the current one so
// the export carries documents from two different months (cert letter).
func writeSAFT(c *ctx, f *fixtures, sw config.SoftwareIdentity, now time.Time) {
	loc := now.Location()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	start := firstOfMonth.AddDate(0, -1, 0)
	end := firstOfMonth.AddDate(0, 1, -1)

	hdr := saft.Header{
		Issuer: f.Issuer,
		Software: saft.SoftwareIdentity{
			ProducerTaxID:     sw.ProducerTaxID,
			CertificateNumber: sw.CertificateNumber,
			ProductID:         sw.ProductID(),
			Version:           sw.Version,
		},
		Start:     start,
		End:       end,
		CreatedAt: now,
	}
	out, err := saft.Export(hdr,
		c.store.snapshotSales(),
		c.store.snapshotStock(),
		c.store.snapshotWork(),
		c.store.snapshotPayments(),
	)
	if err != nil {
		log.Fatalf("saft export: %v", err)
	}

	if err := os.MkdirAll("out", 0o755); err != nil {
		log.Fatalf("mkdir out: %v", err)
	}
	path := filepath.Join("out", fmt.Sprintf("SAFT-DEMO-%s-%s.xml", start.Format("2006-01"), end.Format("2006-01")))
	if err := os.WriteFile(path, out, 0o644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
	fmt.Printf("\nSAF-T written: %s (%d bytes)\n", path, len(out))
}

func mustLisbon() *time.Location {
	loc, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		panic("cannot load Europe/Lisbon: " + err.Error())
	}
	return loc
}
