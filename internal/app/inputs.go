package app

import "github.com/flyzard/invoicing.v2/internal/domain"

// ── enum string consts (re-exported domain values; consumers never import domain) ──
const (
	DocFT = string(domain.FT)
	DocFS = string(domain.FS)
	DocFR = string(domain.FR)
	DocNC = string(domain.NC)
	DocND = string(domain.ND)
	DocGR = string(domain.GR)
	DocGT = string(domain.GT)
	DocGA = string(domain.GA)
	DocGC = string(domain.GC)
	DocGD = string(domain.GD)
	DocOR = string(domain.OR)
	DocPF = string(domain.PF)
	DocNE = string(domain.NE)
	DocCM = string(domain.CM)
	DocFC = string(domain.FC)
	DocFO = string(domain.FO)
	DocOU = string(domain.OU)
	DocRC = string(domain.RC)
	DocRG = string(domain.RG)
)

const (
	RateReduced      = string(domain.TaxReduced)
	RateIntermediate = string(domain.TaxIntermediate)
	RateNormal       = string(domain.TaxNormal)
	RateExempt       = string(domain.TaxExempt)
)

const (
	RegionPT   = string(domain.PT)
	RegionPTAC = string(domain.PTAC)
	RegionPTMA = string(domain.PTMA)
)

const (
	ProductGoods   = string(domain.ProductTypeGoods)
	ProductService = string(domain.ProductTypeService)
	ProductOther   = string(domain.ProductTypeOther)
)

const (
	UnitPiece = string(domain.UnitPiece)
	UnitHour  = string(domain.UnitHour)
)

const (
	MechCash         = string(domain.PaymentMechanismCash)
	MechMultibanco   = string(domain.PaymentMechanismMultibanco)
	MechBankTransfer = string(domain.PaymentMechanismBankTransfer)
	MechCreditCard   = string(domain.PaymentMechanismCreditCard)
	MechDebitCard    = string(domain.PaymentMechanismDebitCard)
	MechCheck        = string(domain.PaymentMechanismCheck)
	MechElectronic   = string(domain.PaymentMechanismElectronic)
	MechOther        = string(domain.PaymentMechanismOther)
)

const (
	ExemptM01 = string(domain.M01)
	ExemptM02 = string(domain.M02)
	ExemptM04 = string(domain.M04)
	ExemptM05 = string(domain.M05)
	ExemptM06 = string(domain.M06)
	ExemptM07 = string(domain.M07)
	ExemptM09 = string(domain.M09)
	ExemptM10 = string(domain.M10)
	ExemptM11 = string(domain.M11)
	ExemptM12 = string(domain.M12)
	ExemptM13 = string(domain.M13)
	ExemptM14 = string(domain.M14)
	ExemptM15 = string(domain.M15)
	ExemptM16 = string(domain.M16)
	ExemptM19 = string(domain.M19)
	ExemptM20 = string(domain.M20)
	ExemptM21 = string(domain.M21)
	ExemptM25 = string(domain.M25)
	ExemptM26 = string(domain.M26)
	ExemptM30 = string(domain.M30)
	ExemptM31 = string(domain.M31)
	ExemptM32 = string(domain.M32)
	ExemptM33 = string(domain.M33)
	ExemptM34 = string(domain.M34)
	ExemptM40 = string(domain.M40)
	ExemptM41 = string(domain.M41)
	ExemptM42 = string(domain.M42)
	ExemptM43 = string(domain.M43)
	ExemptM44 = string(domain.M44)
	ExemptM45 = string(domain.M45)
	ExemptM46 = string(domain.M46)
	ExemptM99 = string(domain.M99)
)

// NOTE: `Idem IdempotencyKey` below refers to the EXISTING IdempotencyKey type in
// invoicing.go — do NOT redeclare it here.

// ── request value types ──
type UserInput struct{ Email, Name string }

type AddressInput struct{ Detail, City, PostalCode string }

type CustomerInput struct {
	TaxID, Name string
	Address     *AddressInput // required unless Anonymous
	Country     string        // ISO; drives the NIF rule
	Anonymous   bool          // explicit; flips FS ceiling + skips address validation
}

type LineTaxInput struct {
	Kind                           string // "VAT" | "NS"
	Region, Category               string
	ExemptionCode, ExemptionReason string
}

type DiscountInput struct {
	Kind        string // "percent" | "amount"
	Percent     float64
	AmountCents int64
}

type DocRefInput struct{ Reference, Reason string }
type OrderRefInput struct{ OriginatingON, OrderDate string } // OrderDate YYYY-MM-DD

type LineInput struct {
	ProductCode, ProductType, ProductDescription, ProductNumberCode, Unit string
	Quantity                                                              float64
	QuantityScaled                                                        int64 // optional ND echo of a prior line; used iff non-zero
	UnitPriceCents                                                        int64
	TaxPointDate                                                          string // YYYY-MM-DD
	Tax                                                                   *LineTaxInput
	Discount                                                              *DiscountInput
	References                                                            []DocRefInput
	OrderReferences                                                       []OrderRefInput
}

type CurrencyInput struct {
	Code      string
	RateMicro int64 // 1.085 → 1085000
}

type FRPaymentInput struct {
	Mechanism   string
	AmountCents int64
	Date        string // YYYY-MM-DD
}

type IssueInvoiceInput struct {
	DocType          string
	SeriesID         string
	SourceID         string
	IssuedBy         UserInput
	Date             string // YYYY-MM-DD
	Customer         CustomerInput
	Lines            []LineInput
	PaymentTermsDays *int
	GlobalDiscount   *DiscountInput
	Currency         *CurrencyInput
	Payments         []FRPaymentInput // FR only
	Idem             IdempotencyKey
}

type IssueWorkInput struct {
	DocType  string
	SeriesID string
	SourceID string
	IssuedBy UserInput
	Date     string
	Customer CustomerInput
	Lines    []LineInput
	Idem     IdempotencyKey
}

type IssueStockInput struct {
	DocType           string
	SeriesID          string
	SourceID          string
	IssuedBy          UserInput
	Date              string
	Customer          CustomerInput
	Lines             []LineInput
	ShipFrom, ShipTo  *AddressInput
	MovementStartTime string // RFC3339 instant
	Idem              IdempotencyKey
}

type TotalsInput struct{ NetCents, TaxCents, GrossCents int64 }

type PaymentMethodInput struct {
	Mechanism   string
	AmountCents int64
	Date        string // YYYY-MM-DD
}

type SourceDocInput struct{ OriginatingON, InvoiceDate, Description string } // InvoiceDate YYYY-MM-DD

type PaymentLineInput struct {
	LineNumber      int
	SourceDocuments []SourceDocInput
	CreditCents     int64
	SettlementCents *int64
	Tax             *LineTaxInput
}

type IssuePaymentInput struct {
	Type            string // DocRC / DocRG
	SeriesID        string
	TransactionDate string // YYYY-MM-DD
	Customer        CustomerInput
	SourceID        string // SAF-T audit field (NOT an issuance-hash sourceID)
	Methods         []PaymentMethodInput
	Lines           []PaymentLineInput
	Totals          TotalsInput
	Idem            IdempotencyKey
}
