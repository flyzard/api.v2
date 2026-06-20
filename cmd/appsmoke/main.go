package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/flyzard/invoicing.v2/internal/adapter/memstore"
	"github.com/flyzard/invoicing.v2/internal/adapter/signing"
	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/config"
)

const (
	tenantID = "smoke-tenant"
	outDir   = "out-appsmoke"
)

// loadSigner builds the real Portaria 363/2010 RSA-SHA1 signer from the producer private key at AT_SIGNING_KEY_FILE.
func loadSigner() (*signing.RSASigner, error) {
	path := os.Getenv("AT_SIGNING_KEY_FILE")
	if path == "" {
		return nil, fmt.Errorf("set AT_SIGNING_KEY_FILE (producer signing key, e.g. at_private.pem)")
	}
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return signing.NewRSASigner(pemBytes, 1)
}

type mapTenantStore struct{ tenants map[string]app.Tenant }

func (s mapTenantStore) Resolve(_ context.Context, id string) (app.Tenant, error) {
	t, ok := s.tenants[id]
	if !ok {
		return app.Tenant{}, app.ErrNotFound
	}
	return t, nil
}

func main() {
	loc := MustLisbon()
	d := func(m, day int) time.Time { return time.Date(2026, time.Month(m), day, 0, 0, 0, 0, loc) }
	// Series are registered before the earliest document (5.1, 8 May 2026).
	seriesReg := time.Date(2026, 4, 1, 9, 0, 0, 0, loc)

	// One clock drives both the scenarios and the app's SystemEntryDate stamping.
	// Each scenario re-pins it (c.atDay) to its own business day at 09:00, so every
	// document's SystemEntryDate lands on its document date.
	clock := NewClock(seriesReg, time.Minute)

	f := BuildFixtures(seriesReg)
	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	sw := cfg.Software

	signer, err := loadSigner()
	if err != nil {
		log.Fatalf("signer: %v", err)
	}

	// App stack backed by memstore. Series are seeded already AT-registered,
	// exactly as the app's own tests do, so issuance has an issuable series.
	store := memstore.New()
	for _, s := range f.Series {
		store.SeedSeries(tenantID, *s)
	}
	tenant := app.Tenant{
		ID:       tenantID,
		Company:  f.Issuer,
		CommMode: app.CommMonthlySAFT, // no outbox/AT-client wiring needed for an issuance smoke
	}
	svc := app.New(app.Deps{
		Tenants:  mapTenantStore{tenants: map[string]app.Tenant{tenantID: tenant}},
		UoW:      store,
		Clock:    clock,
		Signer:   signer,
		Software: sw,
	})

	c := NewCtx(f, clock, NewStore(), svc, tenantID, f.IssuerUser.Email)

	fmt.Println("app-smoke — full cert §5 walkthrough through internal/app (memstore-backed)")
	fmt.Printf("Issuer: %s (NIF %s) · tenant %s\n", f.Issuer.Name, f.Issuer.NIF, tenantID)

	// Dates mirror the reviewed certification dataset; the export spans May–June 2026.
	Scenario51(c, d(5, 8))
	Scenario52(c, d(5, 12))
	Scenario53(c, d(5, 14))
	Scenario54(c, d(5, 15))
	Scenario55(c, d(5, 16))
	Scenario56(c, d(5, 20))
	Scenario57(c, d(6, 3))
	Scenario58(c, d(6, 5))
	Scenario59(c, d(6, 9))
	Scenario510(c, d(6, 10))
	Scenario511(c, d(5, 22))
	Scenario512(c, d(6, 12))
	Scenario513(c)

	// SAF-T comes from the app's ExportService (reads the documents the services
	// persisted in memstore) — proving the all-families export path end-to-end.
	writeAppSAFT(svc, d(5, 1), d(6, 30))
	// PDFs + checklist are pure projections of the issued documents (the app has
	// no multi-via PDF surface), rendered from the same domain values.
	WriteDocumentPDFs(c, sw, outDir)
	WriteChecklist(outDir)

	fmt.Printf("\nDone. Issued %d sales, %d work, %d stock, %d payment documents through internal/app.\n",
		store.SalesCount(), store.WorkCount(), store.StockCount(), store.PaymentCount())
}

// writeAppSAFT exports [start, end] via app.ExportService and writes it under outDir.
func writeAppSAFT(svc *app.Services, start, end time.Time) {
	out, err := svc.Export.ExportSAFT(context.Background(), tenantID, start, end)
	if err != nil {
		log.Fatalf("app ExportSAFT: %v", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", outDir, err)
	}
	path := filepath.Join(outDir, fmt.Sprintf("SAFT-APP-%s-%s.xml", start.Format("2006-01"), end.Format("2006-01")))
	if err := os.WriteFile(path, out, 0o644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
	fmt.Printf("\nSAF-T written (via app.ExportService): %s (%d bytes)\n", path, len(out))
}
