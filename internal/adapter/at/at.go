// Package at implements the client side of three AT webservices: series
// communication (SeriesWS, Portaria 195/2020: registarSerie, finalizarSerie,
// anularSerie, consultarSeries), transport-document communication (sgdtws),
// and real-time invoice communication (fatcorews RegisterInvoice, DL 28/2019).
// Ported from the v1 infrastructure client; wire formats follow the official
// WSDLs and the load-bearing quirks documented inline.
package at

import (
	"context"
	"fmt"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// Error is a failure reported by an AT webservice (connection, HTTP status,
// SOAP fault, or codResultOper >= 3000).
type Error struct {
	Code    string
	Message string
}

func (e Error) Error() string { return fmt.Sprintf("AT error %s: %s", e.Code, e.Message) }

// IsRetryable reports whether the error might succeed on retry.
func (e Error) IsRetryable() bool {
	switch e.Code {
	case "TIMEOUT", "UNAVAILABLE", "BUSY", "CONNECTION",
		"HTTP_502", "HTTP_503", "HTTP_504",
		"9999":
		return true
	}
	return false
}

// SeriesClient is the port for the AT SeriesWS operations. Client (SOAP)
// and NullClient (in-memory fake) implement it.
type SeriesClient interface {
	RegisterSeries(ctx context.Context, req SeriesRegistration) (*SeriesRegistrationResult, error)
	FinalizeSeries(ctx context.Context, req SeriesFinalization) error
	CancelSeries(ctx context.Context, req SeriesCancellation) error
	GetSeriesStatus(ctx context.Context, seriesID string, docType domain.DocumentType) (*SeriesStatus, error)
}

// TransportClient is the port for transport-document communication (sgdtws).
type TransportClient interface {
	CommunicateTransport(ctx context.Context, company domain.Company, mv domain.StockMovement) (*TransportResult, error)
}

// InvoiceClient is the port for real-time invoice communication (fatcorews).
type InvoiceClient interface {
	CommunicateInvoice(ctx context.Context, company domain.Company, inv domain.SalesInvoice) (*InvoiceResult, error)
}

// CancelReasonError is the anularSerie motivo code "Anulação por erro de
// registo" — the only code defined by the AT manual "Comunicação de Séries
// Documentais, Aspetos Específicos" §1.3.10.
const CancelReasonError = "ER"

// SeriesRegistration carries the registarSerie inputs.
type SeriesRegistration struct {
	SeriesID          string
	DocType           domain.DocumentType
	SeriesType        string // tipoSerie: "N" normal, "R" recovery
	InitialSeq        int    // numInicialSeq
	ExpectedStartDate time.Time
}

// SeriesFinalization carries the finalizarSerie inputs.
type SeriesFinalization struct {
	SeriesID      string
	DocType       domain.DocumentType
	ATCode        string // codValidacaoSerie
	LastSeq       int    // seqUltimoDocEmitido
	Justification string // optional, max 4000 chars
}

// SeriesCancellation carries the anularSerie inputs. AT only accepts it for
// a series that never issued a document.
type SeriesCancellation struct {
	SeriesID string
	DocType  domain.DocumentType
	ATCode   string // codValidacaoSerie
	Reason   string // motivo, 2-char code (see CancelReasonError)
}

// SeriesRegistrationResult is AT's answer to registarSerie.
type SeriesRegistrationResult struct {
	ValidationCode   string
	RegistrationDate time.Time
	Status           domain.SeriesATStatus
}

// SeriesStatus is AT's answer to consultarSeries.
type SeriesStatus struct {
	SeriesID       string
	DocType        domain.DocumentType
	ValidationCode string
	Status         domain.SeriesATStatus
	LastSeq        int
}

// RegistrationFor derives the registarSerie request from a series that has
// not yet been registered. tipoSerie follows ProcessingMeans: "N" Normal or
// "R" Recuperação per the AT manual "Comunicação de Séries Documentais,
// Aspetos Específicos" §1.3.6 (Portaria 363/2010 recovery integration).
func RegistrationFor(s domain.Series, startDate time.Time) (SeriesRegistration, error) {
	if s.IsRegistered() {
		return SeriesRegistration{}, fmt.Errorf("series %q is already registered (code %s)", s.ID, s.ATCode)
	}
	seriesType := "N"
	if s.ProcessingMeans == domain.ProcessingRecovery {
		seriesType = "R"
	}
	return SeriesRegistration{
		SeriesID:          s.ID,
		DocType:           s.DocType,
		SeriesType:        seriesType,
		InitialSeq:        s.LastNum + 1,
		ExpectedStartDate: startDate,
	}, nil
}

// FinalizationFor derives the finalizarSerie request from a registered series.
// A series that never issued cannot be finalized (WSDL seqUltimoDocEmitido
// requires >= 1) — cancel it instead (CancellationFor).
func FinalizationFor(s domain.Series, justification string) (SeriesFinalization, error) {
	if !s.IsRegistered() {
		return SeriesFinalization{}, fmt.Errorf("series %q is not registered", s.ID)
	}
	if s.LastNum == 0 {
		return SeriesFinalization{}, fmt.Errorf("series %q never issued a document; cancel it instead", s.ID)
	}
	return SeriesFinalization{
		SeriesID:      s.ID,
		DocType:       s.DocType,
		ATCode:        s.ATCode,
		LastSeq:       s.LastNum,
		Justification: justification,
	}, nil
}

// CancellationFor derives the anularSerie request from a registered series
// that never issued a document.
func CancellationFor(s domain.Series) (SeriesCancellation, error) {
	if !s.IsRegistered() {
		return SeriesCancellation{}, fmt.Errorf("series %q is not registered", s.ID)
	}
	if s.LastNum > 0 {
		return SeriesCancellation{}, fmt.Errorf("series %q issued %d documents; finalize instead", s.ID, s.LastNum)
	}
	return SeriesCancellation{
		SeriesID: s.ID,
		DocType:  s.DocType,
		ATCode:   s.ATCode,
		Reason:   CancelReasonError,
	}, nil
}

// docClass maps a DocumentType to the AT classeDoc code
// (SeriesWS "Aspetos Específicos" §1.3.8).
func docClass(dt domain.DocumentType) (string, error) {
	switch {
	case dt.IsSales():
		return "SI", nil // SalesInvoices
	case dt.IsTransport():
		return "MG", nil // MovementOfGoods
	case dt.IsWorking():
		return "WD", nil // WorkingDocuments
	case dt.IsReceipt():
		return "PY", nil // Payments
	}
	return "", fmt.Errorf("no AT document class for type %q", dt)
}

// statusFromEstado maps the SeriesWS estado code to the domain status.
func statusFromEstado(estado string) domain.SeriesATStatus {
	switch estado {
	case "A":
		return domain.SeriesActive
	case "F":
		return domain.SeriesFinalized
	case "N": // anulada
		return domain.SeriesCancelled
	}
	return domain.SeriesPending
}
