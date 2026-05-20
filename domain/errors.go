package domain

import "errors"

var (
	ErrInvalidTaxID           = errors.New("invalid tax id")
	ErrMissingDocumentType    = errors.New("document type is required")
	ErrMissingCustomer        = errors.New("customer is required")
	ErrNoLines                = errors.New("at least one line is required")
	ErrLineNotFound           = errors.New("line not found")
	ErrSeriesAlreadyRegistered = errors.New("series already registered")
)
