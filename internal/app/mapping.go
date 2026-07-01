package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

var docTypes = map[string]domain.DocumentType{
	DocFT: domain.FT, DocFS: domain.FS, DocFR: domain.FR, DocNC: domain.NC, DocND: domain.ND,
	DocGR: domain.GR, DocGT: domain.GT, DocGA: domain.GA, DocGC: domain.GC, DocGD: domain.GD,
	DocOR: domain.OR, DocPF: domain.PF, DocNE: domain.NE, DocCM: domain.CM, DocFC: domain.FC,
	DocFO: domain.FO, DocOU: domain.OU, DocRC: domain.RC, DocRG: domain.RG,
}

func mapDocType(s string) (domain.DocumentType, *Error) {
	if dt, ok := docTypes[s]; ok {
		return dt, nil
	}
	return "", newErrorCode(KindInvalid, "unknown_doc_type", fmt.Errorf("unknown doc type %q", s))
}

var categories = map[string]domain.TaxCategory{
	RateReduced:      domain.TaxReduced,
	RateIntermediate: domain.TaxIntermediate,
	RateNormal:       domain.TaxNormal,
	RateExempt:       domain.TaxExempt,
}

func mapCategory(s string) (domain.TaxCategory, *Error) {
	if c, ok := categories[s]; ok {
		return c, nil
	}
	return "", newErrorCode(KindInvalid, "unknown_tax_rate", fmt.Errorf("unknown tax rate %q", s))
}

var regions = map[string]domain.TaxRegion{
	RegionPT:   domain.PT,
	RegionPTAC: domain.PTAC,
	RegionPTMA: domain.PTMA,
}

func mapRegion(s string) (domain.TaxRegion, *Error) {
	if r, ok := regions[s]; ok {
		return r, nil
	}
	return "", newErrorCode(KindInvalid, "unknown_tax_region", fmt.Errorf("unknown tax region %q", s))
}

var exemptions = map[string]domain.Exemption{
	ExemptM01: domain.M01,
	ExemptM02: domain.M02,
	ExemptM04: domain.M04,
	ExemptM05: domain.M05,
	ExemptM06: domain.M06,
	ExemptM07: domain.M07,
	ExemptM09: domain.M09,
	ExemptM10: domain.M10,
	ExemptM11: domain.M11,
	ExemptM12: domain.M12,
	ExemptM13: domain.M13,
	ExemptM14: domain.M14,
	ExemptM15: domain.M15,
	ExemptM16: domain.M16,
	ExemptM19: domain.M19,
	ExemptM20: domain.M20,
	ExemptM21: domain.M21,
	ExemptM25: domain.M25,
	ExemptM26: domain.M26,
	ExemptM30: domain.M30,
	ExemptM31: domain.M31,
	ExemptM32: domain.M32,
	ExemptM33: domain.M33,
	ExemptM34: domain.M34,
	ExemptM40: domain.M40,
	ExemptM41: domain.M41,
	ExemptM42: domain.M42,
	ExemptM43: domain.M43,
	ExemptM44: domain.M44,
	ExemptM45: domain.M45,
	ExemptM46: domain.M46,
	ExemptM99: domain.M99,
}

func mapExemption(s string) (domain.Exemption, *Error) {
	if e, ok := exemptions[s]; ok {
		return e, nil
	}
	return "", newErrorCode(KindInvalid, "unknown_exemption", fmt.Errorf("unknown exemption code %q", s))
}

var mechanisms = map[string]domain.PaymentMechanism{
	MechCash:         domain.PaymentMechanismCash,
	MechMultibanco:   domain.PaymentMechanismMultibanco,
	MechBankTransfer: domain.PaymentMechanismBankTransfer,
	MechCreditCard:   domain.PaymentMechanismCreditCard,
	MechDebitCard:    domain.PaymentMechanismDebitCard,
	MechCheck:        domain.PaymentMechanismCheck,
	MechElectronic:   domain.PaymentMechanismElectronic,
	MechOther:        domain.PaymentMechanismOther,
}

func mapMechanism(s string) (domain.PaymentMechanism, *Error) {
	if m, ok := mechanisms[s]; ok {
		return m, nil
	}
	return "", newErrorCode(KindInvalid, "unknown_payment_mechanism", fmt.Errorf("unknown payment mechanism %q", s))
}

var units = map[string]domain.UnitOfMeasure{
	UnitPiece: domain.UnitPiece,
	UnitHour:  domain.UnitHour,
}

func mapUnit(s string) (domain.UnitOfMeasure, *Error) {
	if u, ok := units[s]; ok {
		return u, nil
	}
	return "", newErrorCode(KindInvalid, "unknown_unit", fmt.Errorf("unknown unit %q", s))
}

var productTypes = map[string]domain.ProductType{
	ProductGoods:   domain.ProductTypeGoods,
	ProductService: domain.ProductTypeService,
	ProductOther:   domain.ProductTypeOther,
}

func mapProductType(s string) (domain.ProductType, *Error) {
	if pt, ok := productTypes[s]; ok {
		return pt, nil
	}
	return "", newErrorCode(KindInvalid, "unknown_product_type", fmt.Errorf("unknown product type %q", s))
}

// customerNS is the fixed namespace for deterministic CustomerID derivation.
var customerNS = uuid.MustParse("6f9619ff-8b86-d011-b42d-00c04fc964ff")

var lisbonLoc = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		panic("cannot load Europe/Lisbon: " + err.Error())
	}
	return loc
}()

func moneyFromCents(c int64) domain.Money { return domain.MoneyFromCents(c) }

func rateFromMicro(micro int64) (domain.ExchangeRate, *Error) {
	r, err := domain.NewExchangeRate(float64(micro) / 1e6)
	if err != nil {
		return 0, newErrorCode(KindInvalid, "bad_exchange_rate", err)
	}
	return r, nil
}

func lisbonDate(s string) (time.Time, *Error) {
	t, err := time.ParseInLocation("2006-01-02", s, lisbonLoc)
	if err != nil {
		return time.Time{}, newErrorCode(KindInvalid, "bad_date", fmt.Errorf("date %q: %w", s, err))
	}
	return t, nil
}

func rfc3339(s string) (time.Time, *Error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, newErrorCode(KindInvalid, "bad_timestamp", fmt.Errorf("timestamp %q: %w", s, err))
	}
	return t.In(lisbonLoc), nil
}

func parseNumber(s string) (domain.DocNumber, *Error) {
	n, err := domain.ParseDocNumber(s)
	if err != nil {
		return domain.DocNumber{}, newErrorCode(KindInvalid, "bad_doc_number", fmt.Errorf("doc number %q: %w", s, err))
	}
	return n, nil
}

// lineTaxFrom maps a *LineTaxInput to a domain.LineTax.
// Nil input returns a zero LineTax (legal for non-valued transport lines).
func lineTaxFrom(in *LineTaxInput) (domain.LineTax, *Error) {
	if in == nil {
		return domain.LineTax(nil), nil
	}
	region, rerr := mapRegion(in.Region)
	if rerr != nil {
		return nil, rerr
	}
	switch in.Kind {
	case "NS":
		ex, eerr := mapExemption(in.ExemptionCode)
		if eerr != nil {
			return nil, eerr
		}
		lt, err := domain.NewNotSubjectLineTax(domain.TaxJurisdiction(region), ex, in.ExemptionReason)
		if err != nil {
			return nil, newErrorCode(KindInvalid, "bad_tax", err)
		}
		return lt, nil
	default: // "VAT"
		cat, cerr := mapCategory(in.Category)
		if cerr != nil {
			return nil, cerr
		}
		var ex domain.Exemption
		if in.ExemptionCode != "" {
			e, eerr := mapExemption(in.ExemptionCode)
			if eerr != nil {
				return nil, eerr
			}
			ex = e
		}
		lt, err := domain.NewVATLineTax(region, cat, ex, in.ExemptionReason)
		if err != nil {
			return nil, newErrorCode(KindInvalid, "bad_tax", err)
		}
		return lt, nil
	}
}

func discountFrom(in *DiscountInput) (domain.Discount, *Error) {
	if in == nil {
		return domain.Discount(nil), nil
	}
	if in.Kind == "amount" {
		d, err := domain.NewAmountDiscount(moneyFromCents(in.AmountCents))
		if err != nil {
			return nil, newErrorCode(KindInvalid, "bad_discount", err)
		}
		return d, nil
	}
	d, err := domain.NewPercentDiscount(in.Percent)
	if err != nil {
		return nil, newErrorCode(KindInvalid, "bad_discount", err)
	}
	return d, nil
}

func lineFrom(in LineInput) (domain.DocumentLine, *Error) {
	pt, e := mapProductType(in.ProductType)
	if e != nil {
		return domain.DocumentLine{}, e
	}
	unit, uerr := mapUnit(in.Unit)
	if uerr != nil {
		return domain.DocumentLine{}, uerr
	}
	prod, err := domain.NewProduct(domain.Product{
		ProductCode: in.ProductCode, ProductType: pt, ProductDescription: in.ProductDescription,
		ProductNumberCode: in.ProductNumberCode, Unit: unit, Active: true,
	})
	if err != nil {
		return domain.DocumentLine{}, newErrorCode(KindInvalid, "bad_product", err)
	}
	var qty domain.Quantity
	if in.QuantityScaled != 0 {
		qty = domain.Quantity(in.QuantityScaled)
	} else {
		q, qerr := domain.NewQuantity(in.Quantity)
		if qerr != nil {
			return domain.DocumentLine{}, newErrorCode(KindInvalid, "bad_quantity", qerr)
		}
		qty = q
	}
	tpd, derr := lisbonDate(in.TaxPointDate)
	if derr != nil {
		return domain.DocumentLine{}, derr
	}
	tax, terr := lineTaxFrom(in.Tax)
	if terr != nil {
		return domain.DocumentLine{}, terr
	}
	disc, derr2 := discountFrom(in.Discount)
	if derr2 != nil {
		return domain.DocumentLine{}, derr2
	}
	line := domain.DocumentLine{
		Product: prod, Quantity: qty, UnitPrice: moneyFromCents(in.UnitPriceCents),
		TaxPointDate: tpd, Tax: tax, Discount: disc,
	}
	for _, r := range in.References {
		line.References = append(line.References, domain.DocReference{Reference: r.Reference, Reason: r.Reason})
	}
	for _, o := range in.OrderReferences {
		od, oerr := lisbonDate(o.OrderDate)
		if oerr != nil {
			return domain.DocumentLine{}, oerr
		}
		line.OrderReferences = append(line.OrderReferences, domain.OrderReference{OriginatingON: o.OriginatingON, OrderDate: &od})
	}
	return line, nil
}

func buildCommonDraft(docType string, issuedBy UserInput, date string, customer CustomerInput, lines []LineInput) (domain.CommonDraftDocument, *Error) {
	dt, e := mapDocType(docType)
	if e != nil {
		return domain.CommonDraftDocument{}, e
	}
	cust, e := customerFrom(customer)
	if e != nil {
		return domain.CommonDraftDocument{}, e
	}
	d, e := lisbonDate(date)
	if e != nil {
		return domain.CommonDraftDocument{}, e
	}
	user, uerr := domain.NewUser(issuedBy.Email, issuedBy.Name)
	if uerr != nil {
		return domain.CommonDraftDocument{}, newErrorCode(KindInvalid, "bad_user", uerr)
	}
	cd := domain.CommonDraftDocument{DocumentCore: domain.DocumentCore{DocumentType: dt, Customer: cust, Date: d, IssuedBy: user}}
	for _, li := range lines {
		dl, lerr := lineFrom(li)
		if lerr != nil {
			return domain.CommonDraftDocument{}, lerr
		}
		cd.AddLine(dl)
	}
	return cd, nil
}

func salesDraftFrom(in IssueInvoiceInput) (*domain.DraftSalesInvoice, *Error) {
	cd, e := buildCommonDraft(in.DocType, in.IssuedBy, in.Date, in.Customer, in.Lines)
	if e != nil {
		return nil, e
	}
	date := cd.Date
	draft := &domain.DraftSalesInvoice{CommonDraftDocument: cd}
	if in.PaymentTermsDays != nil {
		due := date.AddDate(0, 0, *in.PaymentTermsDays)
		draft.PaymentTerms = &due
	}
	if in.GlobalDiscount != nil {
		gd, gerr := discountFrom(in.GlobalDiscount)
		if gerr != nil {
			return nil, gerr
		}
		draft.GlobalDiscount = gd
	}
	if in.Currency != nil {
		draft.CalculateTotals()
		rate, rerr := rateFromMicro(in.Currency.RateMicro)
		if rerr != nil {
			return nil, rerr
		}
		code, cerr := domain.NewCurrencyCode(in.Currency.Code)
		if cerr != nil {
			return nil, newErrorCode(KindInvalid, "bad_currency", cerr)
		}
		cur, curerr := domain.NewCurrency(code, draft.Totals.GrossTotal, rate, date)
		if curerr != nil {
			return nil, newErrorCode(KindInvalid, "bad_currency", curerr)
		}
		draft.Currency = &cur
	}
	for _, p := range in.Payments {
		mech, merr := mapMechanism(p.Mechanism)
		if merr != nil {
			return nil, merr
		}
		pd, pderr := lisbonDate(p.Date)
		if pderr != nil {
			return nil, pderr
		}
		draft.Payments = append(draft.Payments, domain.FRPayment{Mechanism: mech, Amount: moneyFromCents(p.AmountCents), Date: pd})
	}
	return draft, nil
}

func workDraftFrom(in IssueWorkInput) (*domain.DraftWorkDocument, *Error) {
	cd, e := buildCommonDraft(in.DocType, in.IssuedBy, in.Date, in.Customer, in.Lines)
	if e != nil {
		return nil, e
	}
	return &domain.DraftWorkDocument{CommonDraftDocument: cd}, nil
}

func stockDraftFrom(in IssueStockInput) (*domain.DraftStockMovement, *Error) {
	cd, e := buildCommonDraft(in.DocType, in.IssuedBy, in.Date, in.Customer, in.Lines)
	if e != nil {
		return nil, e
	}
	draft := &domain.DraftStockMovement{CommonDraftDocument: cd}
	if in.MovementStartTime != "" {
		t, terr := rfc3339(in.MovementStartTime)
		if terr != nil {
			return nil, terr
		}
		draft.MovementStartTime = t
	}
	if in.ShipFrom != nil {
		addr, aerr := domain.NewAddress(in.ShipFrom.Detail, in.ShipFrom.City, in.ShipFrom.PostalCode, domain.Country("PT"))
		if aerr != nil {
			return nil, newErrorCode(KindInvalid, "bad_ship_from", aerr)
		}
		draft.ShipFrom = &domain.ShippingPoint{Address: &addr}
	}
	if in.ShipTo != nil {
		addr, aerr := domain.NewAddress(in.ShipTo.Detail, in.ShipTo.City, in.ShipTo.PostalCode, domain.Country("PT"))
		if aerr != nil {
			return nil, newErrorCode(KindInvalid, "bad_ship_to", aerr)
		}
		draft.ShipTo = &domain.ShippingPoint{Address: &addr}
	}
	return draft, nil
}

func paymentDraftFrom(in IssuePaymentInput) (*domain.PaymentDraft, *Error) {
	dt, e := mapDocType(in.Type)
	if e != nil {
		return nil, e
	}
	cust, e := customerFrom(in.Customer)
	if e != nil {
		return nil, e
	}
	txDate, e := lisbonDate(in.TransactionDate)
	if e != nil {
		return nil, e
	}
	draft := &domain.PaymentDraft{
		Type:            dt,
		TransactionDate: txDate,
		Customer:        cust,
		SourceID:        in.SourceID,
	}
	for _, m := range in.Methods {
		mech, merr := mapMechanism(m.Mechanism)
		if merr != nil {
			return nil, merr
		}
		d, derr := lisbonDate(m.Date)
		if derr != nil {
			return nil, derr
		}
		draft.Methods = append(draft.Methods, domain.PaymentMethod{Mechanism: mech, Amount: moneyFromCents(m.AmountCents), Date: d})
	}
	for _, pl := range in.Lines {
		tax, terr := lineTaxFrom(pl.Tax)
		if terr != nil {
			return nil, terr
		}
		var srcs []domain.SourceDocumentID
		for _, sd := range pl.SourceDocuments {
			inv, iderr := lisbonDate(sd.InvoiceDate)
			if iderr != nil {
				return nil, iderr
			}
			srcs = append(srcs, domain.SourceDocumentID{OriginatingON: sd.OriginatingON, InvoiceDate: inv, Description: sd.Description})
		}
		var settleMoney *domain.Money
		if pl.SettlementCents != nil {
			m := moneyFromCents(*pl.SettlementCents)
			settleMoney = &m
		}
		draft.Lines = append(draft.Lines, domain.PaymentLine{
			LineNumber:       pl.LineNumber,
			SourceDocuments:  srcs,
			SettlementAmount: settleMoney,
			Movement:         domain.CreditAmount{Value: moneyFromCents(pl.CreditCents)},
			Tax:              tax,
		})
	}
	return draft, nil
}

func totalsView(t domain.Totals) TotalsView {
	return TotalsView{
		NetCents:   t.NetTotal.Cents(),
		TaxCents:   t.TaxTotal.Cents(),
		StampCents: t.StampDuty.Cents(),
		GrossCents: t.GrossTotal.Cents(),
		Breakdown:  breakdownView(t.Breakdown),
	}
}

// ── view projectors ───────────────────────────────────────────────────────────

// lineTaxView extracts Region/Category/ExemptionCode strings from a LineTax.
func lineTaxView(t domain.LineTax) (region, category, exemptionCode string) {
	switch v := t.(type) {
	case domain.VATTax:
		return string(v.Rate.Region), string(v.Rate.Category), string(v.Rate.Exemption)
	case domain.NotSubjectTax:
		return string(v.Jurisdiction), "", string(v.Reason)
	case domain.StampTax:
		return string(v.Jurisdiction), "", ""
	}
	return "", "", ""
}

func lineViews(lines []domain.DocumentLine) []LineView {
	out := make([]LineView, len(lines))
	for i, l := range lines {
		r, cat, ex := lineTaxView(l.Tax)
		out[i] = LineView{
			ProductCode:    l.Product.ProductCode,
			Description:    l.Product.ProductDescription,
			Quantity:       float64(l.Quantity) / 100_000,
			QuantityScaled: int64(l.Quantity),
			UnitPriceCents: l.UnitPrice.Cents(),
			Region:         r,
			Category:       cat,
			ExemptionCode:  ex,
		}
	}
	return out
}

func breakdownView(bd domain.TaxBreakdown) []RateBucket {
	out := make([]RateBucket, len(bd))
	for i, e := range bd {
		out[i] = RateBucket{
			Region:               string(e.Region),
			Category:             string(e.Category),
			ExemptionCode:        string(e.ExemptionCode),
			ExemptionDescription: e.ExemptionDescription,
			BaseCents:            e.Base.Cents(),
			TaxCents:             e.Tax.Cents(),
		}
	}
	return out
}

func customerView(c domain.Customer) CustomerView {
	return CustomerView{
		TaxID:     string(c.CustomerTaxID),
		Name:      c.CompanyName,
		Anonymous: c.IsAnonymous(),
	}
}

func currencyView(cur *domain.Currency) *CurrencyView {
	if cur == nil {
		return nil
	}
	return &CurrencyView{
		Code:        string(cur.Code),
		RateMicro:   int64(cur.ExchangeRate),
		AmountCents: cur.Amount.Cents(),
	}
}

// viewFrom projects the common issued-document fields shared by all non-payment families.
func viewFrom(doc domain.IssuedDocument, totals domain.Totals, lines []domain.DocumentLine, currency *domain.Currency) IssuedView {
	n := doc.Number
	date := doc.Date.In(lisbonLoc).Format("2006-01-02")
	statusDate := doc.StatusDate.In(lisbonLoc).Format("2006-01-02")
	return IssuedView{
		Number:        n.Format(),
		Type:          string(n.Type),
		Series:        n.Series,
		Seq:           n.Seq,
		ATCUD:         string(doc.ATCUD),
		Status:        string(doc.Status),
		Date:          date,
		NetCents:      totals.NetTotal.Cents(),
		TaxCents:      totals.TaxTotal.Cents(),
		StampCents:    totals.StampDuty.Cents(),
		GrossCents:    totals.GrossTotal.Cents(),
		Breakdown:     breakdownView(totals.Breakdown),
		Lines:         lineViews(lines),
		Customer:      customerView(doc.Customer),
		Currency:      currencyView(currency),
		Hash:          string(doc.Hash),
		QRPayload:     doc.QRPayload,
		StatusDate:    statusDate,
		Reason:        doc.Reason,
		SourceID:      doc.SourceID,
		SourceBilling: string(doc.SourceBilling),
	}
}

func salesView(doc domain.SalesInvoice) IssuedView {
	return viewFrom(doc.IssuedDocument, doc.Totals, doc.Lines, doc.Currency)
}

func workView(doc domain.WorkDocument) IssuedView {
	return viewFrom(doc.IssuedDocument, doc.Totals, doc.Lines, nil)
}

func stockView(doc domain.StockMovement) IssuedView {
	return viewFrom(doc.IssuedDocument, doc.Totals, doc.Lines, nil)
}

func paymentView(p domain.Payment) IssuedView {
	n := p.Number
	date := p.TransactionDate.In(lisbonLoc).Format("2006-01-02")
	statusDate := p.StatusDate.In(lisbonLoc).Format("2006-01-02")
	return IssuedView{
		Number:     n.Format(),
		Type:       string(n.Type),
		Series:     n.Series,
		Seq:        n.Seq,
		ATCUD:      string(p.ATCUD),
		Status:     string(p.Status),
		Date:       date,
		NetCents:   p.NetTotal.Cents(),
		TaxCents:   p.TaxPayable.Cents(),
		GrossCents: p.GrossTotal.Cents(),
		Customer:   customerView(p.Customer),
		Hash:       "",
		QRPayload:  p.QRPayload,
		StatusDate: statusDate,
		Reason:     p.Reason,
		SourceID:   p.SourceID,
	}
}

func customerFrom(in CustomerInput) (domain.Customer, *Error) {
	if in.Anonymous {
		return domain.NewAnonymousCustomer(), nil
	}
	var key string
	if domain.CustomerTaxID(in.TaxID) == domain.FinalConsumerNIF {
		// shared sentinel: key on identity, not the NIF
		var addr string
		if in.Address != nil {
			addr = in.Address.Detail + in.Address.City + in.Address.PostalCode + in.Country
		}
		key = strings.ToLower(strings.TrimSpace(in.Name)) + "|" + strings.ToLower(strings.TrimSpace(addr))
	} else {
		key = in.Country + ":" + in.TaxID
	}
	if in.Address == nil {
		return domain.Customer{}, newErrorCode(KindInvalid, "missing_address", fmt.Errorf("non-anonymous customer needs an address"))
	}
	addr, err := domain.NewAddress(in.Address.Detail, in.Address.City, in.Address.PostalCode, domain.Country(in.Country))
	if err != nil {
		return domain.Customer{}, newErrorCode(KindInvalid, "bad_address", err)
	}
	c := domain.Customer{
		CustomerID:     uuid.NewSHA1(customerNS, []byte(key)),
		CustomerTaxID:  domain.CustomerTaxID(in.TaxID),
		CompanyName:    in.Name,
		BillingAddress: addr,
	}
	if verr := c.Validate(); verr != nil {
		return domain.Customer{}, newErrorCode(KindInvalid, "invalid_customer", verr)
	}
	return c, nil
}
