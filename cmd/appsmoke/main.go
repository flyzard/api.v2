// Command appsmoke drives the full AT certification §5 walkthrough through the
// internal/app service layer (memstore-backed): every family is issued via
// app.InvoicingService and the SAF-T is produced by app.ExportService. The
// fixtures, the thirteen scenarios, and the PDF/checklist writers are local to
// this binary (fixtures.go, scenarios.go, support.go, artifacts.go); scenarios
// issue through the Ctx helpers (ctx.go), so their bodies never name the app
// layer directly.
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

// loadSigner builds the real Portaria 363/2010 RSA-SHA1 signer from the
// producer private key at AT_SIGNING_KEY_FILE (the same key cmd/atsmoke uses).
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
	today := time.Date(2026, 5, 22, 0, 0, 0, 0, loc)
	clockBase := time.Date(2026, 5, 22, 9, 0, 0, 0, loc)
	prevMonthDay := time.Date(2026, 4, 22, 0, 0, 0, 0, loc)
	prevMonthClock := time.Date(2026, 4, 22, 9, 0, 0, 0, loc)

	// One clock drives both the scenarios and the app's SystemEntryDate stamping,
	// so the April prologue carries April system-entry dates.
	clock := NewClock(prevMonthClock, time.Minute)

	f := BuildFixtures(prevMonthClock)
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

	ScenarioPrevMonth(c, prevMonthDay)
	clock.SetBase(clockBase) // jump from April to May

	Scenario51(c, today)
	Scenario52(c, today)
	Scenario53(c, today)
	Scenario54(c, today)
	Scenario55(c, today)
	Scenario56(c, today)
	Scenario57(c, today)
	Scenario58(c, today)
	Scenario59(c, today)
	Scenario510(c, today)
	Scenario511(c, today)
	Scenario512(c, today)
	Scenario513(c, today)

	// SAF-T comes from the app's ExportService (reads the documents the services
	// persisted in memstore) — proving the all-families export path end-to-end.
	writeAppSAFT(svc, clockBase)
	// PDFs + checklist are pure projections of the issued documents (the app has
	// no multi-via PDF surface), rendered from the same domain values.
	WriteDocumentPDFs(c, sw, outDir)
	WriteChecklist(outDir)

	fmt.Printf("\nDone. Issued %d sales, %d work, %d stock, %d payment documents through internal/app.\n",
		store.SalesCount(), store.WorkCount(), store.StockCount(), store.PaymentCount())
}

// writeAppSAFT exports the period spanning the previous and current month via
// app.ExportService and writes it under outDir.
func writeAppSAFT(svc *app.Services, now time.Time) {
	loc := now.Location()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	start := firstOfMonth.AddDate(0, -1, 0)
	end := firstOfMonth.AddDate(0, 1, -1)

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
