// Command main runs the 13 scenarios required by the AT certification
// checklist (§5.1–5.13). Each scenario issues a real document through the
// domain layer, prints the issued JSON and a one-line summary, and — where
// applicable — simulates downstream artefacts (PDF, SAF-T rows) that live in
// Tier-3 modules not yet implemented.
package main

import (
	"fmt"
	"time"
)

func main() {
	loc := mustLisbon()
	today := time.Date(2026, 5, 22, 0, 0, 0, 0, loc)
	clockBase := time.Date(2026, 5, 22, 9, 0, 0, 0, loc)

	f := buildFixtures(clockBase)
	c := &ctx{
		f:      f,
		signer: stubSigner{},
		clock:  newClock(clockBase, time.Minute),
		store:  newStore(),
	}

	fmt.Println("AT Certification Checklist — §5 walkthrough")
	fmt.Printf("Issuer: %s (NIF %s · EAC %s)\n", f.Issuer.Name, f.Issuer.NIF, f.Issuer.EACCode)
	fmt.Printf("Software: %s %s · cert %s\n", f.Software.ProductID(), f.Software.Version, f.Software.CertificateNumber)
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

	fmt.Println()
	fmt.Println("Done.")
}

func mustLisbon() *time.Location {
	loc, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		panic("cannot load Europe/Lisbon: " + err.Error())
	}
	return loc
}
