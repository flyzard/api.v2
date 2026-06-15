package app

import (
	"context"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// CommMode is the tenant's DL 28/2019 election for how invoices reach AT.
// Zero value is CommRealtime (webservice communication).
type CommMode int

const (
	CommRealtime    CommMode = iota // fatcorews real-time communication
	CommMonthlySAFT                 // SAF-T monthly submission; no per-document comm
)

// ATCredentials are the Portal das Finanças sub-user credentials (WSE permission) the AT client uses for this tenant's webservice calls.
type ATCredentials struct {
	TaxpayerNIF string
	Username    string
	Password    string
}

// Tenant is one issuing company plus what legitimately varies per company.
type Tenant struct {
	ID       string
	Company  domain.Company
	ATCreds  ATCredentials
	CommMode CommMode
	Calendar domain.HolidayCalendar
	FSLimits *domain.FSLimits
}

// TenantStore resolves a tenant by ID (loads company, credentials, config).
type TenantStore interface {
	Resolve(ctx context.Context, tenantID string) (Tenant, error)
}
