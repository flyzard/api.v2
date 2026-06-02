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

	"github.com/flyzard/invoicing.v2/config"
	"github.com/flyzard/invoicing.v2/saft"
)

func main() {
	loc := mustLisbon()
	today := time.Date(2026, 5, 22, 0, 0, 0, 0, loc)
	clockBase := time.Date(2026, 5, 22, 9, 0, 0, 0, loc)

	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatal(err)
	}

	f := buildFixtures(clockBase)
	c := &ctx{
		f:      f,
		signer: stubSigner{},
		clock:  newClock(clockBase, time.Minute),
		store:  newStore(),
	}

	fmt.Println("AT Certification Checklist — §5 walkthrough")
	fmt.Printf("Issuer: %s (NIF %s · EAC %s)\n", f.Issuer.Name, f.Issuer.NIF, f.Issuer.EACCode)
	fmt.Printf("Software: %s %s · cert %s\n", cfg.Software.ProductID(), cfg.Software.Version, cfg.Software.CertificateNumber)
	fmt.Printf("Document date: %s · clock starts at %s\n",
		today.Format("2006-01-02"), clockBase.Format("2006-01-02T15:04 MST"))

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

	fmt.Println()
	fmt.Println("Done.")
}

// writeSAFT projects every recorded document for May 2026 into a SAF-T XML
// file under out/. Phase B wires the call end-to-end; the projector returns
// an empty payload until Phases C–H land.
func writeSAFT(c *ctx, f *fixtures, sw config.SoftwareIdentity, now time.Time) {
	loc := now.Location()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, -1)

	hdr := saft.Header{
		Issuer:    f.Issuer,
		Software:  sw,
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
	path := filepath.Join("out", fmt.Sprintf("SAFT-DEMO-%s.xml", start.Format("2006-01")))
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
