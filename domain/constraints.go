package domain

// XSD-derived field-length limits.
// Source: SAF-T-PT 1.04_01 schema + Portaria 195/2020 + Portaria 363/2010 where applicable.
// Values cross-referenced with regras.md for the address fields.
//
// Adding a new constant: name it after the field it constrains, not the type.
// Replace literal usage at the callsite and surface the constant in the error
// message via %d so a future spec change propagates automatically.
const (
	// Customer / Address (regras.md §customer/billing-address).
	MaxLenCustomerTaxID = 30
	MaxLenAccountID     = 30
	MaxLenAddressDetail = 100
	MaxLenCity          = 50
	MaxLenPostalCode    = 20

	// Line tax (line_tax.go).
	MaxLenStampTaxCode = 10
	MinLenExemptReason = 6
	MaxLenExemptReason = 60

	// Document line / payment (document_line.go, payment.go).
	MaxLenOriginatingON      = 60
	MaxLenReference          = 60
	MaxLenPaymentDescription = 200

	// Issued document chain (atcud.go, hash.go, doc_number.go, issued_document.go).
	MaxLenATCUD              = 100
	MaxLenHash               = 172
	MaxLenHashControl        = 70
	MaxLenDocNumber          = 60
	MaxLenCancellationReason = 100

	// Withholding & shipping (withholding_tax.go, shipping_point.go).
	MaxLenWithholdingDescription = 60
	MaxLenWarehouseID            = 50
	MaxLenLocationID             = 30
	MaxLenDeliveryID             = 200
)
