package domain

import "errors"

var (
	ErrInvalidTaxID           = errors.New("invalid tax id")
	ErrMissingDocumentType    = errors.New("document type is required")
	ErrMissingCustomer        = errors.New("customer is required")
	ErrNoLines                = errors.New("at least one line is required")
	ErrLineNotFound           = errors.New("line not found")
	ErrSeriesAlreadyRegistered = errors.New("series already registered")
	ErrInvalidCountry          = errors.New("invalid country code")
	ErrMissingAddressDetail    = errors.New("address detail is required")
	ErrMissingCity             = errors.New("city is required")
	ErrMissingPostalCode       = errors.New("postal code is required")
	ErrMissingAccountID        = errors.New("account id is required")
	ErrMissingCompanyName      = errors.New("company name is required")
	ErrMissingProductCode      = errors.New("product code is required")
	ErrInvalidProductType      = errors.New("invalid product type")
	ErrMissingProductDescription = errors.New("product description is required")
	ErrMissingProductNumberCode  = errors.New("product number code is required")
	ErrInvalidUnit             = errors.New("invalid unit of measure")
)
