package domain

import (
	"encoding/json"
	"fmt"
)

// DocumentType is sealed: its only possible values are the constants below
// or the result of ParseDocumentType. The underlying field is unexported so
// no package outside domain can construct one directly (e.g. DocumentType("XX")
// does not compile) — external code must go through ParseDocumentType/JSON
// unmarshaling, both of which validate against documentTypes.
type DocumentType struct{ code string }

func (dt DocumentType) String() string { return dt.code }

func (dt DocumentType) MarshalJSON() ([]byte, error) { return json.Marshal(dt.code) }

func (dt *DocumentType) UnmarshalJSON(data []byte) error {
	return unmarshalString(data, ParseDocumentType, dt)
}

// ParseDocumentType validates s against the known document types.
func ParseDocumentType(s string) (DocumentType, error) {
	dt := DocumentType{code: s}
	if !dt.IsValid() {
		return DocumentType{}, fmt.Errorf("%w: %q", ErrInvalidDocumentType, s)
	}
	return dt, nil
}

var (
	// Sales
	FT = DocumentType{"FT"}
	FS = DocumentType{"FS"}
	FR = DocumentType{"FR"}
	NC = DocumentType{"NC"}
	ND = DocumentType{"ND"}

	// Transport
	GT = DocumentType{"GT"}
	GR = DocumentType{"GR"}
	GA = DocumentType{"GA"}
	GC = DocumentType{"GC"}
	GD = DocumentType{"GD"}

	// Working
	OR = DocumentType{"OR"}
	PF = DocumentType{"PF"}
	NE = DocumentType{"NE"}
	CM = DocumentType{"CM"}
	FC = DocumentType{"FC"}
	FO = DocumentType{"FO"}
	OU = DocumentType{"OU"}

	// Receipts
	RC = DocumentType{"RC"}
	RG = DocumentType{"RG"}
)

// docFamily groups DocumentTypes by SAF-T family.
type docFamily string

const (
	familySales     docFamily = "sales"
	familyTransport docFamily = "transport"
	familyWorking   docFamily = "working"
	familyReceipt   docFamily = "receipt"
)

// docTypeRules captures the SAF-T family classification plus per-doctype business rules that the validator enforces uniformly.
type docTypeRules struct {
	Family          docFamily
	RequiresRef     bool // every line must carry a DocReference (AT rule for NC/ND)
	AllowsStamp     bool // line may carry StampTax (false for transport: XSD MovementTax restriction)
	RequiresLineTax bool // every line must carry Tax (sales/working per XSD; transport has its own valued-guia rule)
}

var documentTypes = map[DocumentType]docTypeRules{
	FT: {Family: familySales, AllowsStamp: true, RequiresLineTax: true},
	FS: {Family: familySales, AllowsStamp: true, RequiresLineTax: true},
	FR: {Family: familySales, AllowsStamp: true, RequiresLineTax: true},
	NC: {Family: familySales, AllowsStamp: true, RequiresRef: true, RequiresLineTax: true},
	ND: {Family: familySales, AllowsStamp: true, RequiresRef: true, RequiresLineTax: true},

	GT: {Family: familyTransport},
	GR: {Family: familyTransport},
	GA: {Family: familyTransport},
	GC: {Family: familyTransport},
	GD: {Family: familyTransport},

	OR: {Family: familyWorking, AllowsStamp: true, RequiresLineTax: true},
	PF: {Family: familyWorking, AllowsStamp: true, RequiresLineTax: true},
	NE: {Family: familyWorking, AllowsStamp: true, RequiresLineTax: true},
	CM: {Family: familyWorking, AllowsStamp: true, RequiresLineTax: true},
	FC: {Family: familyWorking, AllowsStamp: true, RequiresLineTax: true},
	FO: {Family: familyWorking, AllowsStamp: true, RequiresLineTax: true},
	OU: {Family: familyWorking, AllowsStamp: true, RequiresLineTax: true},

	RC: {Family: familyReceipt},
	RG: {Family: familyReceipt},
}

func (dt DocumentType) IsZero() bool  { return dt == DocumentType{} }
func (dt DocumentType) IsValid() bool { _, ok := documentTypes[dt]; return ok }
func (dt DocumentType) IsSales() bool { return documentTypes[dt].Family == familySales }

// IsFactura narrows IsSales() to the fatura subset (FT/FS/FR)
func (dt DocumentType) IsFactura() bool {
	switch dt {
	case FT, FS, FR:
		return true
	}
	return false
}
func (dt DocumentType) IsTransport() bool { return documentTypes[dt].Family == familyTransport }
func (dt DocumentType) IsWorking() bool   { return documentTypes[dt].Family == familyWorking }
func (dt DocumentType) IsReceipt() bool   { return documentTypes[dt].Family == familyReceipt }
