package saft

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
	"github.com/google/uuid"
	"golang.org/x/text/encoding/charmap"
)

// fixedHash is a 172-char base64 string of the same shape stubSigner emits —
// exercises the projector's verbatim Hash passthrough without dragging the
// cmd-local stubSigner into this package.
const fixedHash = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func minimalSalesInvoice() domain.SalesInvoice {
	custID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	prod := must(domain.NewProduct(domain.Product{
		ProductCode:        "P-NOR",
		ProductType:        domain.ProductTypeGoods,
		ProductDescription: "Auriculares Bluetooth",
		ProductNumberCode:  "P-NOR",
		Unit:               domain.UnitPiece,
	}))
	num := must(domain.NewDocNumber(domain.FT, "FT2026", 1))
	tax := must(domain.NewVATLineTax(domain.PT, domain.TaxNormal, "", ""))
	date := time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)
	return domain.SalesInvoice{
		IssuedDocument: domain.IssuedDocument{
			Number:          num,
			ATCUD:           "AAAAAAAA-1",
			Hash:            fixedHash,
			HashControl:     "1",
			SystemEntryDate: date,
			SourceID:        "issuer@test",
			SourceBilling:   domain.SourceBillingProduced,
			Status:          domain.StatusNormal,
			StatusDate:      date,
			DocumentCore: domain.DocumentCore{
				DocumentType: domain.FT,
				Customer: domain.Customer{
					CustomerID:    custID,
					AccountID:     "ACC-001",
					CustomerTaxID: "500000000",
					CompanyName:   "Acme Faturação Lda.",
					BillingAddress: domain.Address{
						AddressDetail: "Rua das Flores 1",
						City:          "Lisboa",
						PostalCode:    "1000-001",
						Country:       "PT",
					},
				},
				Date: date,
				Lines: []domain.DocumentLine{{
					LineNumber:   1,
					Product:      prod,
					Quantity:     must(domain.NewQuantity(2)),
					UnitPrice:    must(domain.NewMoney(50.00)),
					TaxPointDate: date,
					Tax:          tax,
				}},
				Totals: domain.Totals{
					NetTotal:   must(domain.NewMoney(100.00)),
					TaxTotal:   must(domain.NewMoney(23.00)),
					GrossTotal: must(domain.NewMoney(123.00)),
				},
			},
		},
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func decodeWin1252(t *testing.T, out []byte) string {
	t.Helper()
	utf8, err := charmap.Windows1252.NewDecoder().Bytes(out)
	if err != nil {
		t.Fatalf("Win-1252 → UTF-8 decode: %v", err)
	}
	return string(utf8)
}

func TestExport_StructureIntact(t *testing.T) {
	inv := minimalSalesInvoice()
	hdr := Header{
		Issuer: domain.Company{
			NIF:        "500000000",
			Name:       "Test Issuer Lda.",
			FiscalYear: 2026,
			Address: domain.Address{
				AddressDetail: "Rua de Teste 1",
				City:          "Lisboa",
				PostalCode:    "1000-001",
				Country:       "PT",
			},
		},
		Software: SoftwareIdentity{
			ProducerTaxID:     "519348761",
			CertificateNumber: "9999",
			ProductID:         "Test/Producer",
			Version:           "1.0.0",
		},
		Start:     time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
	}

	out, err := Export(hdr, []domain.SalesInvoice{inv}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Decoding via the Windows-1252 charset so the test catches regressions
	// in the encoding pipeline, not only struct shapes.
	body := decodeWin1252(t, out)

	wants := []string{
		`<?xml version="1.0" encoding="Windows-1252"?>`,
		`<AuditFile xmlns="urn:OECD:StandardAuditFile-Tax:PT_1.04_01">`,
		`<InvoiceNo>FT FT2026/1</InvoiceNo>`,
		`<ATCUD>AAAAAAAA-1</ATCUD>`,
		`<Hash>` + fixedHash + `</Hash>`,
		`<HashControl>1</HashControl>`,
		`<NumberOfEntries>1</NumberOfEntries>`,
		`<TotalCredit>100.00</TotalCredit>`,
		`<NetTotal>100.00</NetTotal>`,
		`<GrossTotal>123.00</GrossTotal>`,
		`<TaxPayable>23.00</TaxPayable>`,
		`<TaxPercentage>23.00</TaxPercentage>`,
		`<CompanyName>Acme Faturação Lda.</CompanyName>`,
		`<UnitPrice>50.00000</UnitPrice>`,
		`<CreditAmount>100.00000</CreditAmount>`,
		`<Quantity>2</Quantity>`,
	}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in output", w)
		}
	}
}

func TestExport_Win1252Bytes(t *testing.T) {
	inv := minimalSalesInvoice()
	hdr := Header{Issuer: domain.Company{
		NIF:     "500000000",
		Name:    "Faturação",
		Address: domain.Address{AddressDetail: "x", City: "x", PostalCode: "1000-001", Country: "PT"},
	}}

	out, err := Export(hdr, []domain.SalesInvoice{inv}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// "Faturação" → 'F','a','t','u','r','a',0xE7 (ç),0xE3 (ã),'o' in Win-1252.
	// In UTF-8 'ç' would be the two bytes 0xC3 0xA7.
	if !bytes.Contains(out, []byte{'F', 'a', 't', 'u', 'r', 'a', 0xE7, 0xE3, 'o'}) {
		t.Errorf("Portuguese characters not Win-1252-encoded in output")
	}
	if bytes.Contains(out, []byte{0xC3, 0xA7}) {
		t.Errorf("UTF-8 byte sequence 0xC3 0xA7 found — transcode left UTF-8 in output")
	}
}

// TestExport_ProductDescriptionDrift covers the AT cert §5.6/§5.10 (round 3348)
// failure mode: two invoices reuse the same ProductCode but with differing
// ProductDescriptions, which AT rejects because MasterFiles/Product and the
// Line description must reconcile. Export must surface this as an error rather
// than silently picking first-seen.
func TestExport_ProductDescriptionDrift(t *testing.T) {
	inv1 := minimalSalesInvoice()
	inv2 := minimalSalesInvoice()
	// Drift only on the product description; same ProductCode "P-NOR".
	inv2.Lines[0].Product.ProductDescription = "Auriculares Bluetooth PRO"
	// Different doc number so the dedup sort key differs.
	inv2.Number = must(domain.NewDocNumber(domain.FT, "FT2026", 2))

	_, err := Export(Header{}, []domain.SalesInvoice{inv1, inv2}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected Export to reject ProductDescription drift, got nil error")
	}
	if !strings.Contains(err.Error(), "P-NOR") {
		t.Errorf("error should name the conflicting ProductCode, got: %v", err)
	}
}

// TestExport_CurrencyDirection pins the math direction in buildCurrency.
// SAF-T CurrencyAmount = invoice gross in the original (foreign) currency;
// our domain stores Amount in EUR and ExchangeRate as foreign-per-EUR
// (e.g. 1.085 USD per 1 EUR), so the wire emission must be Amount × Rate.
// Regression guard: if anyone flips the multiplication, large-currency
// invoices silently mis-report by orders of magnitude.
func TestExport_CurrencyDirection(t *testing.T) {
	inv := minimalSalesInvoice()
	// Single line: 4 × €80 = €320 gross net (no tax on this fixture).
	inv.Lines[0].Quantity = must(domain.NewQuantity(4))
	inv.Lines[0].UnitPrice = must(domain.NewMoney(80.00))
	inv.Totals.NetTotal = must(domain.NewMoney(320.00))
	inv.Totals.TaxTotal = must(domain.NewMoney(73.60))
	inv.Totals.GrossTotal = must(domain.NewMoney(393.60))
	currency := must(domain.NewCurrency(
		must(domain.NewCurrencyCode("USD")),
		must(domain.NewMoney(320.00)),       // EUR amount
		must(domain.NewExchangeRate(1.085)), // USD per EUR
		inv.Date,
	))
	inv.Currency = &currency

	out, err := Export(Header{}, []domain.SalesInvoice{inv}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	body := decodeWin1252(t, out)
	wants := []string{
		`<CurrencyCode>USD</CurrencyCode>`,
		`<CurrencyAmount>347.20</CurrencyAmount>`, // 320.00 × 1.085 = 347.20
		`<ExchangeRate>1.085000</ExchangeRate>`,
	}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in output", w)
		}
	}
}

// TestExport_ControlSumsExcludeCancelledAndBilled pins Portaria 302/2016
// fields 4.1.1–4.1.3: NumberOfEntries counts every invoice, while TotalDebit/
// TotalCredit exclude InvoiceStatus "A" and "F".
func TestExport_ControlSumsExcludeCancelledAndBilled(t *testing.T) {
	normal := minimalSalesInvoice()
	cancelled := minimalSalesInvoice()
	cancelled.Number = must(domain.NewDocNumber(domain.FT, "FT2026", 2))
	cancelled.Status = domain.StatusCancelled
	billed := minimalSalesInvoice()
	billed.Number = must(domain.NewDocNumber(domain.FT, "FT2026", 3))
	billed.Status = domain.StatusBilled

	out, err := Export(Header{}, []domain.SalesInvoice{normal, cancelled, billed}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	body := decodeWin1252(t, out)

	for _, w := range []string{
		`<NumberOfEntries>3</NumberOfEntries>`,
		`<TotalDebit>0.00</TotalDebit>`,
		`<TotalCredit>100.00</TotalCredit>`, // only the Normal invoice
	} {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in output", w)
		}
	}
}

// TestExport_WorkingSumsExcludeCancelledOnly pins Portaria 302/2016 fields
// 4.3.2/4.3.3: only WorkStatus "A" leaves the sums — "F" (faturado) work
// documents stay counted, unlike SalesInvoices.
func TestExport_WorkingSumsExcludeCancelledOnly(t *testing.T) {
	mk := func(seq int, status domain.DocumentStatus) domain.WorkDocument {
		inv := minimalSalesInvoice()
		w := domain.WorkDocument{IssuedDocument: inv.IssuedDocument}
		w.DocumentType = domain.OR
		w.Number = must(domain.NewDocNumber(domain.OR, "OR2026", seq))
		w.Status = status
		return w
	}

	out, err := Export(Header{}, nil, nil, []domain.WorkDocument{
		mk(1, domain.StatusNormal),
		mk(2, domain.StatusCancelled),
		mk(3, domain.StatusBilled),
	}, nil)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	body := decodeWin1252(t, out)

	for _, w := range []string{
		`<NumberOfEntries>3</NumberOfEntries>`,
		`<TotalCredit>200.00</TotalCredit>`, // Normal + Billed; Cancelled out
	} {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in output", w)
		}
	}
}

// TestExport_MovementQuantityExcludesCancelled pins Portaria 302/2016 fields
// 4.2.1/4.2.2: NumberOfMovementLines counts every line, TotalQuantityIssued
// excludes lines of MovementStatus "A" documents.
func TestExport_MovementQuantityExcludesCancelled(t *testing.T) {
	mk := func(seq int, status domain.DocumentStatus) domain.StockMovement {
		inv := minimalSalesInvoice()
		m := domain.StockMovement{IssuedDocument: inv.IssuedDocument}
		m.DocumentType = domain.GT
		m.Number = must(domain.NewDocNumber(domain.GT, "GT2026", seq))
		m.Status = status
		return m
	}

	out, err := Export(Header{}, nil, []domain.StockMovement{
		mk(1, domain.StatusNormal),    // qty 2
		mk(2, domain.StatusCancelled), // qty 2, excluded from the sum
	}, nil, nil)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	body := decodeWin1252(t, out)

	for _, w := range []string{
		`<NumberOfMovementLines>2</NumberOfMovementLines>`,
		`<TotalQuantityIssued>2</TotalQuantityIssued>`,
	} {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in output", w)
		}
	}
}

// TestExport_CustomerIDFitsXSDAndPeriodOmitted pins two schema rules:
// CustomerID must fit SAFPTtextTypeMandatoryMax30Car (≤30 chars — the raw
// 36-char UUID form does not), and the optional Period element is omitted
// (it means "month of the taxation period", which the domain doesn't model).
func TestExport_CustomerIDFitsXSDAndPeriodOmitted(t *testing.T) {
	inv := minimalSalesInvoice()
	out, err := Export(Header{}, []domain.SalesInvoice{inv}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	body := decodeWin1252(t, out)

	id := saftCustomerID(inv.Customer.CustomerID)
	if len(id) > 30 {
		t.Errorf("saftCustomerID too long for XSD: %d chars (%q)", len(id), id)
	}
	// Same key in MasterFiles/Customer and in the document — referential
	// integrity per Portaria 302/2016 field 4.1.4.14.
	if got := strings.Count(body, `<CustomerID>`+id+`</CustomerID>`); got != 2 {
		t.Errorf("CustomerID %q occurrences = %d, want 2 (master + invoice)", id, got)
	}
	if strings.Contains(body, `<Period>`) {
		t.Error("Period element present — should be omitted")
	}
}

// TestExport_AnonymousCustomerAddressDesconhecido pins Portaria 302/2016
// 2.2.6.x: address fields of "Consumidor final" operations carry the literal
// "Desconhecido".
func TestExport_AnonymousCustomerAddressDesconhecido(t *testing.T) {
	inv := minimalSalesInvoice()
	inv.Customer = domain.NewAnonymousCustomer()

	out, err := Export(Header{}, []domain.SalesInvoice{inv}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	body := decodeWin1252(t, out)

	for _, w := range []string{
		`<CustomerTaxID>999999990</CustomerTaxID>`,
		`<CompanyName>Consumidor final</CompanyName>`,
		`<AddressDetail>Desconhecido</AddressDetail>`,
		`<City>Desconhecido</City>`,
		`<PostalCode>Desconhecido</PostalCode>`,
		`<Country>Desconhecido</Country>`,
	} {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in output", w)
		}
	}
}

// TestExport_TaxAccountingBasis pins the Header basis: default "F", explicit
// "S" for the per-supplier autofaturação file (Portaria 302/2016 alínea g),
// anything else rejected before emission.
func TestExport_TaxAccountingBasis(t *testing.T) {
	inv := minimalSalesInvoice()

	out, err := Export(Header{}, []domain.SalesInvoice{inv}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !strings.Contains(decodeWin1252(t, out), `<TaxAccountingBasis>F</TaxAccountingBasis>`) {
		t.Error("default basis should be F")
	}

	out, err = Export(Header{TaxAccountingBasis: "S"}, []domain.SalesInvoice{inv}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Export basis S: %v", err)
	}
	if !strings.Contains(decodeWin1252(t, out), `<TaxAccountingBasis>S</TaxAccountingBasis>`) {
		t.Error("explicit S basis not emitted")
	}

	if _, err = Export(Header{TaxAccountingBasis: "C"}, []domain.SalesInvoice{inv}, nil, nil, nil); err == nil {
		t.Error("basis C should be rejected")
	}
}

// TestBuildTax covers the three LineTax variants and pins the field shape
// against the XSD assert for sales lines (TaxAmount-absent OR TaxAmount/
// TaxExemptionReason XOR — see saftpt1.04_01.xsd:432). Movement lines use
// TaxPercentage in place of TaxAmount; the same logic applies symmetrically.
func TestBuildTax(t *testing.T) {
	vatNormal := must(domain.NewVATLineTax(domain.PT, domain.TaxNormal, "", ""))
	vatExempt := must(domain.NewVATLineTax(domain.PT, domain.TaxExempt, domain.M07, "Isento artigo 9.º CIVA"))
	jur := must(domain.NewTaxJurisdiction("PT"))
	stamp := domain.StampTax{Jurisdiction: jur, Code: "20.01", Amount: must(domain.NewMoney(1.50))}
	notSubj := domain.NotSubjectTax{Jurisdiction: jur, Reason: domain.M99, ReasonText: "Não sujeito"}

	cases := []struct {
		name       string
		tax        domain.LineTax
		wantType   string
		wantPctSet bool
		wantAmtSet bool
		wantExempt bool
	}{
		{"vat-normal", vatNormal, "IVA", true, false, false},
		{"vat-exempt", vatExempt, "IVA", true, false, true},
		{"stamp", stamp, "IS", false, true, false},
		{"not-subject", notSubj, "NS", true, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tx, reason, code := buildTax(c.tax)
			if tx == nil {
				t.Fatal("nil Tax")
			}
			if tx.TaxType != c.wantType {
				t.Errorf("TaxType = %q, want %q", tx.TaxType, c.wantType)
			}
			if (tx.TaxPercentage != "") != c.wantPctSet {
				t.Errorf("TaxPercentage presence = %v (%q), want %v", tx.TaxPercentage != "", tx.TaxPercentage, c.wantPctSet)
			}
			if (tx.TaxAmount != "") != c.wantAmtSet {
				t.Errorf("TaxAmount presence = %v (%q), want %v", tx.TaxAmount != "", tx.TaxAmount, c.wantAmtSet)
			}
			if (reason != "" && code != "") != c.wantExempt {
				t.Errorf("exemption presence = %v (reason=%q code=%q), want %v", reason != "" && code != "", reason, code, c.wantExempt)
			}
		})
	}
}
