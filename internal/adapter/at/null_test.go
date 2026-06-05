package at

import (
	"context"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

func nullReg() SeriesRegistration {
	return SeriesRegistration{
		SeriesID: "S2026", DocType: domain.FT, SeriesType: "N", InitialSeq: 1, ExpectedStartDate: atT0,
	}
}

func TestNullRegisterIsDeterministic(t *testing.T) {
	c := NewNullClient()
	ctx := context.Background()

	r1, err := c.RegisterSeries(ctx, nullReg())
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := domain.ValidateATCode(r1.ValidationCode); err != nil {
		t.Fatalf("generated code invalid: %v", err)
	}
	r2, err := c.RegisterSeries(ctx, nullReg())
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if r1.ValidationCode != r2.ValidationCode {
		t.Errorf("codes differ on repeat registration: %q vs %q", r1.ValidationCode, r2.ValidationCode)
	}
}

func TestNullCodesUniquePerSeries(t *testing.T) {
	c := NewNullClient()
	ctx := context.Background()

	r1, _ := c.RegisterSeries(ctx, nullReg())
	other := nullReg()
	other.DocType = domain.NC
	r2, err := c.RegisterSeries(ctx, other)
	if err != nil {
		t.Fatalf("register NC: %v", err)
	}
	if r1.ValidationCode == r2.ValidationCode {
		t.Error("different series must get different codes")
	}
}

func TestNullRegisterValidatesSeriesID(t *testing.T) {
	c := NewNullClient()
	bad := nullReg()
	bad.SeriesID = "AT-OOPS"
	if _, err := c.RegisterSeries(context.Background(), bad); err == nil {
		t.Fatal("want validation error for series id starting with AT")
	}
}

func TestNullLifecycle(t *testing.T) {
	c := NewNullClient()
	ctx := context.Background()

	r, _ := c.RegisterSeries(ctx, nullReg())

	st, err := c.GetSeriesStatus(ctx, "S2026", domain.FT)
	if err != nil || st.Status != domain.SeriesActive {
		t.Fatalf("status = %+v, %v", st, err)
	}

	if err := c.FinalizeSeries(ctx, SeriesFinalization{
		SeriesID: "S2026", DocType: domain.FT, ATCode: r.ValidationCode, LastSeq: 3,
	}); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	st, _ = c.GetSeriesStatus(ctx, "S2026", domain.FT)
	if st.Status != domain.SeriesFinalized || st.LastSeq != 3 {
		t.Errorf("after finalize: %+v", st)
	}
}

func TestNullCancel(t *testing.T) {
	c := NewNullClient()
	ctx := context.Background()

	r, _ := c.RegisterSeries(ctx, nullReg())
	if err := c.CancelSeries(ctx, SeriesCancellation{
		SeriesID: "S2026", DocType: domain.FT, ATCode: r.ValidationCode, Reason: CancelReasonError,
	}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	st, _ := c.GetSeriesStatus(ctx, "S2026", domain.FT)
	if st.Status != domain.SeriesCancelled {
		t.Errorf("after cancel: %+v", st)
	}
}

func TestNullUnknownSeries(t *testing.T) {
	c := NewNullClient()
	if _, err := c.GetSeriesStatus(context.Background(), "NOPE", domain.FT); err == nil {
		t.Fatal("want error for unknown series")
	}
	if err := c.FinalizeSeries(context.Background(), SeriesFinalization{
		SeriesID: "NOPE", DocType: domain.FT, ATCode: "BCDFGH37",
	}); err == nil {
		t.Fatal("want error finalizing unknown series")
	}
}

func TestNullCommunicateTransport(t *testing.T) {
	c := NewNullClient()
	res, err := c.CommunicateTransport(context.Background(), testCompany(t), testMovement(t))
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	if res.ATDocCodeID == "" {
		t.Fatal("fake must return a non-empty ATDocCodeID")
	}
	res2, _ := c.CommunicateTransport(context.Background(), testCompany(t), testMovement(t))
	if res.ATDocCodeID != res2.ATDocCodeID {
		t.Error("fake ATDocCodeID must be deterministic per document number")
	}
}

func TestNullCommunicateInvoice(t *testing.T) {
	c := NewNullClient()
	res, err := c.CommunicateInvoice(context.Background(), testCompany(t), testInvoice(t))
	if err != nil {
		t.Fatalf("invoice: %v", err)
	}
	if res.Code != 0 {
		t.Errorf("Code = %d, want 0", res.Code)
	}
}
