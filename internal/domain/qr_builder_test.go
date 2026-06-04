package domain

import (
	"strings"
	"testing"
	"time"
)

func eur(t *testing.T, v float64) Money {
	t.Helper()
	m, err := NewMoney(v)
	if err != nil {
		t.Fatalf("NewMoney(%v): %v", v, err)
	}
	return m
}

func qty(t *testing.T, v float64) Quantity {
	t.Helper()
	q, err := NewQuantity(v)
	if err != nil {
		t.Fatalf("NewQuantity(%v): %v", v, err)
	}
	return q
}

// hashAt builds a Hash whose 1-based positions 1,11,21,31 hold the given runes,
// so field Q resolves to want. Filler 'x' elsewhere.
func hashAt(p1, p11, p21, p31 byte) Hash {
	b := []byte("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx") // 32 chars
	b[0], b[10], b[20], b[30] = p1, p11, p21, p31
	return Hash(b)
}

func ptCustomer() Customer {
	return Customer{
		CustomerTaxID:  "539915661",
		BillingAddress: Address{Country: "PT"},
	}
}

func TestBuildQRPayload_Goldens(t *testing.T) {
	cfg := QRConfig{IssuerNIF: "570023262", CertificateNumber: "9999"}

	tests := []struct {
		name string
		doc  IssuedDocument
		cfg  QRConfig
		want string
	}{
		{
			// AT-corrected message, round 1 (FT A/3). All of I2..I8 populated.
			name: "round1_FT_A3",
			doc: IssuedDocument{
				Number: DocNumber{Type: FT, Series: "A", Seq: 3},
				ATCUD:  "BCDFGH37-3",
				Hash:   hashAt('I', 'P', 'K', 'T'),
				Status: StatusNormal,
				DocumentCore: DocumentCore{
					DocumentType: FT,
					Customer:     ptCustomer(),
					Date:         time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC),
					Totals: Totals{
						TaxTotal:      eur(t, 15.39),
						GrossTotal:    eur(t, 201.39),
						AmountPayable: eur(t, 201.39),
						Breakdown: TaxBreakdown{
							{Region: PT, Category: TaxExempt, Base: eur(t, 100.00)},
							{Region: PT, Category: TaxReduced, Base: eur(t, 18.50), Tax: eur(t, 1.11)},
							{Region: PT, Category: TaxIntermediate, Base: eur(t, 12.50), Tax: eur(t, 1.63)},
							{Region: PT, Category: TaxNormal, Base: eur(t, 55.00), Tax: eur(t, 12.65)},
						},
					},
				},
			},
			cfg:  cfg,
			want: "A:570023262*B:539915661*C:PT*D:FT*E:N*F:20260417*G:FT A/3*H:BCDFGH37-3*I1:PT*I2:100.00*I3:18.50*I4:1.11*I5:12.50*I6:1.63*I7:55.00*I8:12.65*N:15.39*O:201.39*Q:IPKT*R:9999",
		},
		{
			// AT-corrected message, round 2 (cancelled FT A/1). Built with the
			// issuance status N; only I7/I8 non-zero, so I2..I6 are omitted.
			name: "round2_cancelled_FT_A1",
			doc: IssuedDocument{
				Number: DocNumber{Type: FT, Series: "A", Seq: 1},
				ATCUD:  "BCDFGH37-1",
				Hash:   hashAt('r', 'j', 'X', 'W'),
				Status: StatusNormal,
				DocumentCore: DocumentCore{
					DocumentType: FT,
					Customer:     ptCustomer(),
					Date:         time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
					Totals: Totals{
						TaxTotal:      eur(t, 5.52),
						GrossTotal:    eur(t, 29.52),
						AmountPayable: eur(t, 29.52),
						Breakdown: TaxBreakdown{
							{Region: PT, Category: TaxNormal, Base: eur(t, 24.00), Tax: eur(t, 5.52)},
						},
					},
				},
			},
			cfg:  cfg,
			want: "A:570023262*B:539915661*C:PT*D:FT*E:N*F:20260426*G:FT A/1*H:BCDFGH37-1*I1:PT*I7:24.00*I8:5.52*N:5.52*O:29.52*Q:rjXW*R:9999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildQRPayload(&tt.doc, tt.cfg)
			if got != tt.want {
				t.Errorf("buildQRPayload mismatch:\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}

// baseDoc is a minimal valid PT FT for variant cases; tweak per test.
func baseDoc(t *testing.T) IssuedDocument {
	t.Helper()
	return IssuedDocument{
		Number: DocNumber{Type: FT, Series: "A", Seq: 1},
		ATCUD:  "AT-1",
		Hash:   hashAt('a', 'b', 'c', 'd'),
		Status: StatusNormal,
		DocumentCore: DocumentCore{
			DocumentType: FT,
			Customer:     ptCustomer(),
			Date:         time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			Totals: Totals{
				TaxTotal:      eur(t, 23.00),
				GrossTotal:    eur(t, 123.00),
				AmountPayable: eur(t, 123.00),
				Breakdown: TaxBreakdown{
					{Region: PT, Category: TaxNormal, Base: eur(t, 100.00), Tax: eur(t, 23.00)},
				},
			},
		},
	}
}

func TestBuildQRPayload_Variants(t *testing.T) {
	cfg := QRConfig{IssuerNIF: "500000000", CertificateNumber: "1234"}

	t.Run("multi_region_ordering_and_omission", func(t *testing.T) {
		d := baseDoc(t)
		d.Totals.Breakdown = TaxBreakdown{
			{Region: PT, Category: TaxNormal, Base: eur(t, 100.00), Tax: eur(t, 23.00)},
			{Region: PTAC, Category: TaxReduced, Base: eur(t, 50.00), Tax: eur(t, 2.00)},
			{Region: PTMA, Category: TaxExempt, Base: eur(t, 10.00)},
		}
		got := buildQRPayload(&d, cfg)
		// I (PT) before J (PT-AC) before K (PT-MA); each block fully emitted.
		wantBlocks := "I1:PT*I7:100.00*I8:23.00*J1:PT-AC*J3:50.00*J4:2.00*K1:PT-MA*K2:10.00"
		if !containsInOrder(got, "I1:PT", "J1:PT-AC", "K1:PT-MA") {
			t.Errorf("region block order wrong: %s", got)
		}
		if !strings.Contains(got, wantBlocks) {
			t.Errorf("region blocks:\n got: %s\nwant substring: %s", got, wantBlocks)
		}
	})

	t.Run("out_only_region_emits_region_code", func(t *testing.T) {
		d := baseDoc(t)
		// OUT category has no QR sub-field but PT still occupies a fiscal space,
		// and its tax still counts toward N (TaxPayable) and O.
		d.Totals.Breakdown = TaxBreakdown{
			{Region: PT, Category: TaxOther, Base: eur(t, 100.00), Tax: eur(t, 5.00)},
		}
		d.Totals.TaxTotal = eur(t, 5.00)
		d.Totals.GrossTotal = eur(t, 105.00)
		d.Totals.AmountPayable = eur(t, 105.00)
		got := buildQRPayload(&d, cfg)
		if !strings.Contains(got, "*I1:PT*") {
			t.Errorf("OUT-only PT must still emit I1:PT: %s", got)
		}
		if strings.Contains(got, "I2:") || strings.Contains(got, "I7:") {
			t.Errorf("OUT has no rate sub-field: %s", got)
		}
		// OUT tax has no I-field but still appears in N.
		if !strings.Contains(got, "*N:5.00*") {
			t.Errorf("OUT tax must count toward N: %s", got)
		}
	})

	t.Run("empty_region_absent", func(t *testing.T) {
		d := baseDoc(t) // PT only
		got := buildQRPayload(&d, cfg)
		if strings.Contains(got, "J1:") || strings.Contains(got, "K1:") {
			t.Errorf("expected no J/K blocks: %s", got)
		}
	})

	t.Run("withholding_P_present", func(t *testing.T) {
		d := baseDoc(t)
		d.Totals.AmountPayable = eur(t, 110.00) // gross 123 - 13 withheld
		got := buildQRPayload(&d, cfg)
		if !strings.Contains(got, "*P:13.00*") {
			t.Errorf("expected P:13.00: %s", got)
		}
	})

	t.Run("withholding_P_omitted_when_zero", func(t *testing.T) {
		d := baseDoc(t) // AmountPayable == GrossTotal
		got := buildQRPayload(&d, cfg)
		if strings.Contains(got, "*P:") {
			t.Errorf("expected no P field: %s", got)
		}
	})

	t.Run("stamp_M_present_not_in_L_or_I", func(t *testing.T) {
		d := baseDoc(t)
		d.Totals.StampDuty = eur(t, 2.00) // baseDoc TaxTotal=23.00
		got := buildQRPayload(&d, cfg)
		if !strings.Contains(got, "*M:2.00*") {
			t.Errorf("expected M:2.00: %s", got)
		}
		// N = TaxPayable = VAT 23.00 + stamp 2.00.
		if !strings.Contains(got, "*N:25.00*") {
			t.Errorf("expected N:25.00 (VAT+stamp): %s", got)
		}
		if strings.Contains(got, "*L:") {
			t.Errorf("stamp must not appear as L: %s", got)
		}
	})

	t.Run("non_subject_base_L", func(t *testing.T) {
		d := baseDoc(t)
		d.Lines = []DocumentLine{
			{
				Quantity:  qty(t, 1),
				UnitPrice: eur(t, 40.00),
				Tax:       NotSubjectTax{Jurisdiction: "PT", Reason: M99, ReasonText: "Não sujeito"},
			},
		}
		got := buildQRPayload(&d, cfg)
		if !strings.Contains(got, "*L:40.00*") {
			t.Errorf("expected L:40.00: %s", got)
		}
	})

	t.Run("nc_amounts_positive_no_minus", func(t *testing.T) {
		d := baseDoc(t)
		d.DocumentType = NC
		d.Number = DocNumber{Type: NC, Series: "A", Seq: 1}
		got := buildQRPayload(&d, cfg)
		if strings.Contains(got, ":-") {
			t.Errorf("NC must not emit negative amounts: %s", got)
		}
		if !strings.Contains(got, "D:NC") {
			t.Errorf("expected D:NC: %s", got)
		}
	})

	t.Run("non_valued_guia_no_vat_block", func(t *testing.T) {
		d := baseDoc(t)
		d.DocumentType = GT
		d.Number = DocNumber{Type: GT, Series: "A", Seq: 1}
		d.Totals = Totals{TaxTotal: 0, GrossTotal: 0, AmountPayable: 0} // empty breakdown
		got := buildQRPayload(&d, cfg)
		// Spec rule (g)/(h): at least one fiscal space must exist; a no-VAT-rate
		// document uses region code "0".
		if !strings.Contains(got, "*I1:0*") {
			t.Errorf("non-valued guia must emit I1:0: %s", got)
		}
		if strings.Contains(got, "I2:") || strings.Contains(got, "I7:") {
			t.Errorf("non-valued guia must have no rate sub-fields: %s", got)
		}
		if !strings.Contains(got, "*N:0.00*") {
			t.Errorf("expected N:0.00: %s", got)
		}
	})

	t.Run("third_party_E_T", func(t *testing.T) {
		d := baseDoc(t)
		d.Status = StatusThirdParty
		got := buildQRPayload(&d, cfg)
		if !strings.Contains(got, "*E:T*") {
			t.Errorf("expected E:T: %s", got)
		}
	})

	t.Run("self_billed_E_S", func(t *testing.T) {
		d := baseDoc(t)
		d.Status = StatusSelfBilled
		got := buildQRPayload(&d, cfg)
		if !strings.Contains(got, "*E:S*") {
			t.Errorf("expected E:S: %s", got)
		}
	})

	t.Run("other_info_S_strips_only_asterisk", func(t *testing.T) {
		d := baseDoc(t)
		c := cfg
		c.OtherInfo = "a*b:c" // only "*" is forbidden in S; ":" is allowed
		got := buildQRPayload(&d, c)
		if !strings.Contains(got, "*S:ab:c") {
			t.Errorf("expected S:ab:c (only * stripped): %s", got)
		}
	})

	t.Run("other_info_S_omitted_when_empty", func(t *testing.T) {
		d := baseDoc(t)
		got := buildQRPayload(&d, cfg)
		if strings.Contains(got, "*S:") {
			t.Errorf("expected no S field: %s", got)
		}
	})

	t.Run("Q_positions_not_first4", func(t *testing.T) {
		d := baseDoc(t)
		// 172-char hash; positions 1,11,21,31 spell "WXYZ", first-4 are "AAAA".
		b := make([]byte, 172)
		for i := range b {
			b[i] = 'A'
		}
		b[0], b[10], b[20], b[30] = 'W', 'X', 'Y', 'Z'
		d.Hash = Hash(b)
		got := buildQRPayload(&d, cfg)
		if !strings.Contains(got, "*Q:WXYZ*") {
			t.Errorf("expected Q:WXYZ (positions 1,11,21,31), got: %s", got)
		}
	})

	t.Run("country_fallback_desconhecido", func(t *testing.T) {
		d := baseDoc(t)
		d.Customer.BillingAddress.Country = ""
		got := buildQRPayload(&d, cfg)
		if !strings.Contains(got, "*C:Desconhecido*") {
			t.Errorf("expected C:Desconhecido: %s", got)
		}
	})
}

// TestQRPayload_FrozenAcrossStatusChange asserts the F-QR-3 invariant: once set
// at issuance, QRPayload is never recomputed when the status later flips to A
// (Cancel) or F (MarkBilled). Field E in the QR therefore keeps its issuance
// value — the exact bug AT flagged in round 2 (a leaked E:F on a cancelled doc).
func TestQRPayload_FrozenAcrossStatusChange(t *testing.T) {
	const sentinel = "A:500*B:1*C:PT*D:FT*E:N*Q:abcd*R:1"
	recent := time.Date(2026, 6, 1, 9, 0, 0, 0, lisbonLocation)

	t.Run("cancel_does_not_mutate_qr", func(t *testing.T) {
		d := IssuedDocument{
			Status:       StatusNormal,
			QRPayload:    sentinel,
			DocumentCore: DocumentCore{Date: recent},
		}
		if err := d.Cancel("Erro de emissão", recent); err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		if d.Status != StatusCancelled {
			t.Fatalf("status = %q, want A", d.Status)
		}
		if d.QRPayload != sentinel {
			t.Errorf("QRPayload mutated by Cancel:\n got: %s\nwant: %s", d.QRPayload, sentinel)
		}
	})

	t.Run("markbilled_does_not_mutate_qr", func(t *testing.T) {
		w := WorkDocument{IssuedDocument: IssuedDocument{
			Status:       StatusNormal,
			QRPayload:    sentinel,
			DocumentCore: DocumentCore{Date: recent},
		}}
		if err := w.MarkBilled(DocNumber{Type: FT, Series: "A", Seq: 1}, recent); err != nil {
			t.Fatalf("MarkBilled: %v", err)
		}
		if w.Status != StatusBilled {
			t.Fatalf("status = %q, want F", w.Status)
		}
		if w.QRPayload != sentinel {
			t.Errorf("QRPayload mutated by MarkBilled:\n got: %s\nwant: %s", w.QRPayload, sentinel)
		}
	})
}

// containsInOrder reports whether each sub appears in s, in order.
func containsInOrder(s string, subs ...string) bool {
	from := 0
	for _, sub := range subs {
		i := strings.Index(s[from:], sub)
		if i < 0 {
			return false
		}
		from += i + len(sub)
	}
	return true
}
