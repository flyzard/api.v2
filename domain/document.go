package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type DraftDocument interface {
	Validate() error
	SetType(doctype DocumentType) error
	AddLine(line DocumentLine) error
	RemoveLine(lineID uint8) error
	UpdateLine(lineID uint8, line DocumentLine) error
	CalculateTotals() error
	SetCustomer(customer Customer) error
	SetSeries(series Series) error
	SetDate(date time.Time) error
}

type Totals struct {
	NetTotal   Money `json:"net_total"`
	TaxTotal   Money `json:"tax_total"`
	StampDuty  Money `json:"stamp_duty"`
	GrossTotal Money `json:"gross_total"`
}

type CommonDraftDocument struct {
	DocumentType DocumentType   `json:"doc_type"`
	Customer     Customer       `json:"customer"`
	Date         time.Time      `json:"date"`
	IssuedBy     User           `json:"issued_by"`
	Series       Series         `json:"series"`
	Lines        []DocumentLine `json:"lines"`
}

func (d *CommonDraftDocument) Validate() error {
	if d.DocumentType == "" {
		return ErrMissingDocumentType
	}
	if d.Customer.CustomerID == uuid.Nil {
		return ErrMissingCustomer
	}
	if len(d.Lines) == 0 {
		return ErrNoLines
	}
	for i, line := range d.Lines {
		if err := line.Validate(); err != nil {
			return fmt.Errorf("line %d: %w", i, err)
		}
	}
	return nil
}

func (d *CommonDraftDocument) SetType(doctype DocumentType) error {
	d.DocumentType = doctype
	return nil
}

func (d *CommonDraftDocument) AddLine(line DocumentLine) error {
	d.Lines = append(d.Lines, line)
	return nil
}

func (d *CommonDraftDocument) RemoveLine(index uint8) error {
	if index >= uint8(len(d.Lines)) {
		return fmt.Errorf("%w: index out of range: %d", ErrLineNotFound, index)
	}
	d.Lines = append(d.Lines[:index], d.Lines[index+1:]...)
	return nil
}

func (d *CommonDraftDocument) UpdateLine(index uint8, line DocumentLine) error {
	if index >= uint8(len(d.Lines)) {
		return fmt.Errorf("%w: index out of range: %d", ErrLineNotFound, index)
	}
	d.Lines[index] = line
	return nil
}

func (d *CommonDraftDocument) SetSeries(series Series) error {
	d.Series = series
	return nil
}

func (d *CommonDraftDocument) SetDate(date time.Time) error {
	d.Date = date
	return nil
}

func (d *CommonDraftDocument) CalculateTotals() error {
	for _ := range d.Lines {
	}
	return nil
}

func (d *CommonDraftDocument) SetCustomer(customer Customer) error {
	d.Customer = customer

	return nil
}

func (d *CommonDraftDocument) NetTotal() error {
	return nil
}

type DraftFS struct {
	CommonDraftDocument
}
