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
	RemoveLine(lineID int8) error
	UpdateLine(lineID int8, line DocumentLine) error
	CalculateTotals() error
	SetCustomer(customer Customer) error
	SetSeries(series Series) error
	SetDate(date time.Time) error
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
	if d.Customer.ID == uuid.Nil {
		return ErrMissingCustomer
	}
	if len(d.Lines) == 0 {
		return ErrNoLines
	}
	return nil
}

func (d *CommonDraftDocument) SetType(doctype DocumentType) error {
	d.DocumentType = doctype
	return nil
}

func (d *CommonDraftDocument) AddLine(line DocumentLine) error {
	if len(d.Lines) == 0 {
		line.ID = 1
	} else {
		last := d.Lines[len(d.Lines)-1]
		line.ID = last.ID + 1
	}

	d.Lines = append(d.Lines, line)
	return nil
}

func (d *CommonDraftDocument) findLineIndex(lineID int16) (int, error) {
	if lineID < 0 {
		return -1, fmt.Errorf("%w: id cannot be negative: %d", ErrLineNotFound, lineID)
	}
	if lineID >= int16(len(d.Lines)) {
		return -1, fmt.Errorf("%w: id out of range: %d", ErrLineNotFound, lineID)
	}
	for i, line := range d.Lines {
		if line.ID == lineID {
			return i, nil
		}
	}
	return -1, fmt.Errorf("%w: id %d", ErrLineNotFound, lineID)
}

func (d *CommonDraftDocument) RemoveLine(lineID int16) error {
	i, err := d.findLineIndex(lineID)
	if err != nil {
		return err
	}
	d.Lines = append(d.Lines[:i], d.Lines[i+1:]...)
	return nil
}

func (d *CommonDraftDocument) UpdateLine(lineID int16, line DocumentLine) error {
	i, err := d.findLineIndex(lineID)
	if err != nil {
		return err
	}
	d.Lines[i] = line
	return nil
}

type DraftFS struct {
	CommonDraftDocument
}

func (d *DraftFS) SetCustomer(customer Customer) error {
	d.Customer = customer

	return nil
}
