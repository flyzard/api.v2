// Command main runs the 13 scenarios required by the AT certification
// checklist (§5.1–5.13). Each scenario issues a real document through the
// domain layer, prints the issued JSON and a one-line summary, and — where
// applicable — simulates downstream artefacts (PDF, SAF-T rows). The scenarios
// themselves live in internal/certkit and are shared with cmd/appsmoke; this
// binary wires a certkit.DomainIssuer (straight to the domain layer).
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/flyzard/invoicing.v2/internal/adapter/signing"
	"github.com/flyzard/invoicing.v2/internal/certkit"
	"github.com/flyzard/invoicing.v2/internal/config"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

func main() {
	loc := certkit.MustLisbon()
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
	f := certkit.BuildFixtures(prevMonthClock)
	clock := certkit.NewClock(prevMonthClock, time.Minute)
	qr := domain.QRConfig{
		IssuerNIF:         f.Issuer.NIF,
		CertificateNumber: cfg.Software.CertificateNumber,
	}
	iss := certkit.NewDomainIssuer(f, clock, signer, qr)
	c := certkit.NewCtx(f, clock, certkit.NewStore(), iss)

	fmt.Println("AT Certification Checklist — §5 walkthrough")
	fmt.Printf("Issuer: %s (NIF %s · EAC %s)\n", f.Issuer.Name, f.Issuer.NIF, f.Issuer.EACCode)
	fmt.Printf("Software: %s %s · cert %s\n", cfg.Software.ProductID(), cfg.Software.Version, cfg.Software.CertificateNumber)
	fmt.Printf("Document dates: %s (mês anterior) · %s (§5) · clock starts at %s\n",
		prevMonthDay.Format("2006-01-02"), today.Format("2006-01-02"), prevMonthClock.Format("2006-01-02T15:04 MST"))
	fmt.Printf("Signer: %s\n", signerName)

	certkit.ScenarioPrevMonth(c, prevMonthDay)
	clock.SetBase(clockBase) // jump from April to May

	certkit.Scenario51(c, today)
	certkit.Scenario52(c, today)
	certkit.Scenario53(c, today)
	certkit.Scenario54(c, today)
	certkit.Scenario55(c, today)
	certkit.Scenario56(c, today)
	certkit.Scenario57(c, today)
	certkit.Scenario58(c, today)
	certkit.Scenario59(c, today)
	certkit.Scenario510(c, today)
	certkit.Scenario511(c, today)
	certkit.Scenario512(c, today)
	certkit.Scenario513(c, today)

	certkit.WriteSAFT(c, cfg.Software, today, "out")
	certkit.WriteDocumentPDFs(c, cfg.Software, "out")
	certkit.WriteChecklist("out")

	fmt.Println()
	fmt.Println("Done.")
}
