package app

// IssuedView is the value-out read-model, sized to the richest reader (the 5.13
// cross-document reads + a future GET endpoint). See spec §6.
type IssuedView struct {
	Number                                     string // canonical "FT FT2026/3"
	Type, Series                               string
	Seq                                        int
	ATCUD, Status                              string
	Date                                       string // YYYY-MM-DD
	NetCents, TaxCents, StampCents, GrossCents int64
	Breakdown                                  []RateBucket
	Lines                                      []LineView
	Customer                                   CustomerView
	Currency                                   *CurrencyView
	Hash                                       string
	QRPayload                                  string
	// cancellation read-model
	StatusDate, Reason, SourceID, SourceBilling string
}

type RateBucket struct {
	Region, Category, ExemptionCode, ExemptionDescription string
	BaseCents, TaxCents                                   int64
}

type LineView struct {
	ProductCode, Description        string
	Quantity                        float64
	QuantityScaled                  int64
	UnitPriceCents                  int64
	Region, Category, ExemptionCode string
}

type CustomerView struct {
	TaxID, Name string
	Anonymous   bool
}

type CurrencyView struct {
	Code        string
	RateMicro   int64
	AmountCents int64
}

type TotalsView struct {
	NetCents, TaxCents, StampCents, GrossCents int64
	Breakdown                                  []RateBucket
}
