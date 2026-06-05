package main

import (
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// TestBuildNCDraftIssueOffline verifies that buildNCDraft produces a valid draft
// that IssueSalesInvoice accepts when referencing a locally-issued FT. No
// network calls are made.
func TestBuildNCDraftIssueOffline(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)

	custAddr, err := domain.NewAddress("Av Dois 2", "Porto", "4000-002", "PT")
	if err != nil {
		t.Fatalf("customer address: %v", err)
	}
	customer, err := domain.NewCustomer(
		"SMOKE1",
		domain.CustomerTaxID("555555550"),
		"Cliente Smoke",
		custAddr,
		false,
	)
	if err != nil {
		t.Fatalf("customer: %v", err)
	}

	signer := stubSigner{}
	qrCfg := domain.QRConfig{IssuerNIF: domain.TaxID("500000000"), CertificateNumber: "0"}

	// Issue a local FT first so we have a real Number to reference.
	ftSeries, err := domain.NewSeries("FTSMOKE01", domain.FT)
	if err != nil {
		t.Fatalf("new FT series: %v", err)
	}
	if err := ftSeries.RegisterWithAT("BCDFGH37", now); err != nil {
		t.Fatalf("register FT series: %v", err)
	}
	ftDraft, err := buildFTDraft(*customer, ftSeries, now)
	if err != nil {
		t.Fatalf("buildFTDraft: %v", err)
	}
	ftInv, err := domain.IssueSalesInvoice(ftDraft, &ftSeries, signer, "smoke@faturly.pt", now, domain.IssueOptions{}, qrCfg)
	if err != nil {
		t.Fatalf("IssueSalesInvoice FT: %v", err)
	}
	t.Logf("FT issued: %s", ftInv.Number.Format())

	// Now build and issue the NC referencing the FT.
	ncSeries, err := domain.NewSeries("NCSMOKE01", domain.NC)
	if err != nil {
		t.Fatalf("new NC series: %v", err)
	}
	if err := ncSeries.RegisterWithAT("BCDFGH39", now); err != nil {
		t.Fatalf("register NC series: %v", err)
	}
	ncDraft, err := buildNCDraft(*customer, ncSeries, ftInv, now)
	if err != nil {
		t.Fatalf("buildNCDraft: %v", err)
	}
	ncInv, err := domain.IssueSalesInvoice(ncDraft, &ncSeries, signer, "smoke@faturly.pt", now, domain.IssueOptions{}, qrCfg)
	if err != nil {
		t.Fatalf("IssueSalesInvoice NC: %v", err)
	}
	if ncInv.Number.Format() == "" {
		t.Error("expected non-empty NC document number")
	}
	if string(ncInv.ATCUD) == "" {
		t.Error("expected non-empty NC ATCUD")
	}
	t.Logf("NC issued: %s ATCUD=%s gross=%s", ncInv.Number.Format(), ncInv.ATCUD, ncInv.Totals.GrossTotal.Format2DP())
}

// TestBuildDraftsIssueOffline verifies that buildFTDraft and buildGTDraft produce
// valid drafts that IssueSalesInvoice and IssueStockMovement accept without any
// network calls. Uses a locally-registered fake series (RegisterWithAT with a
// placeholder code) so the entire issuance path is exercised offline.
func TestBuildDraftsIssueOffline(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)

	custAddr, err := domain.NewAddress("Av Dois 2", "Porto", "4000-002", "PT")
	if err != nil {
		t.Fatalf("customer address: %v", err)
	}
	customer, err := domain.NewCustomer(
		"SMOKE1",
		domain.CustomerTaxID("555555550"),
		"Cliente Smoke",
		custAddr,
		false,
	)
	if err != nil {
		t.Fatalf("customer: %v", err)
	}

	signer := stubSigner{}
	qrCfg := domain.QRConfig{IssuerNIF: domain.TaxID("500000000"), CertificateNumber: "0"}

	t.Run("FT", func(t *testing.T) {
		ftSeries, err := domain.NewSeries("FTSMOKE01", domain.FT)
		if err != nil {
			t.Fatalf("new series: %v", err)
		}
		if err := ftSeries.RegisterWithAT("BCDFGH37", now); err != nil {
			t.Fatalf("register series: %v", err)
		}
		draft, err := buildFTDraft(*customer, ftSeries, now)
		if err != nil {
			t.Fatalf("buildFTDraft: %v", err)
		}
		inv, err := domain.IssueSalesInvoice(draft, &ftSeries, signer, "smoke@faturly.pt", now, domain.IssueOptions{}, qrCfg)
		if err != nil {
			t.Fatalf("IssueSalesInvoice: %v", err)
		}
		if inv.Number.Format() == "" {
			t.Error("expected non-empty document number")
		}
		if string(inv.ATCUD) == "" {
			t.Error("expected non-empty ATCUD")
		}
		t.Logf("FT issued: %s ATCUD=%s gross=%s", inv.Number.Format(), inv.ATCUD, inv.Totals.GrossTotal.Format2DP())
	})

	t.Run("GT", func(t *testing.T) {
		gtSeries, err := domain.NewSeries("GTSMOKE01", domain.GT)
		if err != nil {
			t.Fatalf("new series: %v", err)
		}
		if err := gtSeries.RegisterWithAT("BCDFGH38", now); err != nil {
			t.Fatalf("register series: %v", err)
		}
		draft, err := buildGTDraft(*customer, gtSeries, now)
		if err != nil {
			t.Fatalf("buildGTDraft: %v", err)
		}
		mv, err := domain.IssueStockMovement(draft, &gtSeries, signer, "smoke@faturly.pt", now, domain.IssueOptions{}, qrCfg)
		if err != nil {
			t.Fatalf("IssueStockMovement: %v", err)
		}
		if mv.Number.Format() == "" {
			t.Error("expected non-empty document number")
		}
		if string(mv.ATCUD) == "" {
			t.Error("expected non-empty ATCUD")
		}
		t.Logf("GT issued: %s ATCUD=%s gross=%s", mv.Number.Format(), mv.ATCUD, mv.Totals.GrossTotal.Format2DP())
	})
}
