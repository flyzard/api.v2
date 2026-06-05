// Command atsmoke exercises the AT SeriesWS *test* environment end-to-end:
// register a throwaway series, consult it, then cancel it (anularSerie) so
// the test environment is left clean. Requires a Portal das Finanças
// sub-user with the WSE (series webservice) operation permission.
//
// Env (via .env or environment), names matching the v1 app:
//
//	AT_NIF, AT_USERNAME, AT_PASSWORD   sub-user credentials
//	AT_PUBLIC_KEY_FILE                 AT cipher cert/key PEM ("Chave Cifra Publica AT")
//	AT_CLIENT_CERT_FILE, AT_CLIENT_KEY_FILE  TesteWebservices client TLS pair (test env)
//	AT_CERT_NUM                        numCertSWFatur (default 0)
//	AT_TEST_LOG_BODIES=1               dump SOAP XML (passwords masked)
//	AT_TEST_COMM_ENABLED=1             run document communication smoke (fatcorews + sgdtws)
package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/flyzard/invoicing.v2/internal/adapter/at"
	"github.com/flyzard/invoicing.v2/internal/config"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

func main() {
	if _, err := config.Load(".env"); err != nil { // merges .env into the environment
		log.Fatalf("config: %v", err)
	}

	nif := os.Getenv("AT_NIF")
	user := os.Getenv("AT_USERNAME")
	pass := os.Getenv("AT_PASSWORD")
	if nif == "" || user == "" || pass == "" {
		log.Fatal("set AT_NIF, AT_USERNAME and AT_PASSWORD (Portal das Finanças sub-user with the WSE series permission)")
	}

	keyFile := os.Getenv("AT_PUBLIC_KEY_FILE")
	if keyFile == "" {
		log.Fatal("set AT_PUBLIC_KEY_FILE (AT cipher certificate, e.g. certs/at-public-key.pem)")
	}
	pemData, err := os.ReadFile(keyFile)
	if err != nil {
		log.Fatalf("read %s: %v", keyFile, err)
	}
	atPub, err := at.ParseATPublicKey(string(pemData))
	if err != nil {
		log.Fatalf("parse AT public key: %v", err)
	}

	var clientCert tls.Certificate
	certFile, certKey := os.Getenv("AT_CLIENT_CERT_FILE"), os.Getenv("AT_CLIENT_KEY_FILE")
	if certFile != "" && certKey != "" {
		clientCert, err = tls.LoadX509KeyPair(certFile, certKey)
		if err != nil {
			log.Fatalf("load client TLS pair: %v", err)
		}
	}

	certNum := os.Getenv("AT_CERT_NUM")
	logBodies := os.Getenv("AT_TEST_LOG_BODIES") == "1"
	if logBodies {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}

	client, err := at.NewClient(at.Config{
		SeriesURL:       at.TestSeriesURL,
		TransportURL:    at.TestTransportURL,
		InvoiceURL:      at.TestInvoiceURL,
		TaxpayerNIF:     nif,
		Username:        user,
		Password:        pass,
		SoftwareCertNum: certNum, // "" defaults to "0" (uncertified)
		ATPublicKey:     atPub,
		Certificate:     clientCert,
		LogBodies:       logBodies,
	})
	if err != nil {
		log.Fatalf("client: %v", err)
	}

	ctx := context.Background()
	now := time.Now()

	// Throwaway series, unique per run so reruns don't collide.
	seriesID := "SMK" + now.Format("0601021504")
	series, err := domain.NewSeries(seriesID, domain.FT)
	if err != nil {
		log.Fatalf("new series: %v", err)
	}

	fmt.Printf("→ registarSerie %s (FT) on %s\n", seriesID, at.TestSeriesURL)
	reg, err := at.RegistrationFor(series, now)
	if err != nil {
		log.Fatalf("registration request: %v", err)
	}
	res, err := client.RegisterSeries(ctx, reg)
	if err != nil {
		log.Fatalf("RegisterSeries: %v", err)
	}
	fmt.Printf("  codValidacaoSerie=%s estado=%s dataRegisto=%s\n",
		res.ValidationCode, res.Status, res.RegistrationDate.Format("2006-01-02"))

	if err := series.RegisterWithAT(res.ValidationCode, res.RegistrationDate); err != nil {
		log.Fatalf("RegisterWithAT: %v", err)
	}
	atcud, err := domain.NewATCUD(series, 1)
	if err != nil {
		log.Fatalf("ATCUD preview: %v", err)
	}
	fmt.Printf("  first-document ATCUD would be %s\n", atcud)

	fmt.Printf("→ consultarSeries %s\n", seriesID)
	st, err := client.GetSeriesStatus(ctx, seriesID, domain.FT)
	if err != nil {
		log.Fatalf("GetSeriesStatus: %v", err)
	}
	fmt.Printf("  estado=%s lastSeq=%d code=%s\n", st.Status, st.LastSeq, st.ValidationCode)

	fmt.Printf("→ anularSerie %s (cleanup, motivo=ER)\n", seriesID)
	canc, err := at.CancellationFor(series)
	if err != nil {
		log.Fatalf("cancellation request: %v", err)
	}
	if err := client.CancelSeries(ctx, canc); err != nil {
		log.Fatalf("CancelSeries: %v", err)
	}
	if err := series.Cancel(time.Now()); err != nil {
		log.Fatalf("domain Cancel: %v", err)
	}

	st, err = client.GetSeriesStatus(ctx, seriesID, domain.FT)
	if err != nil {
		log.Fatalf("GetSeriesStatus after cancel: %v", err)
	}
	fmt.Printf("  estado=%s (expect %s)\n", st.Status, domain.SeriesCancelled)

	fmt.Println("Done: register → consult → cancel round-trip OK against AT test environment.")

	if os.Getenv("AT_TEST_COMM_ENABLED") == "1" {
		if err := runDocCommSmoke(ctx, client, now); err != nil {
			log.Fatalf("doc-comm smoke: %v", err)
		}
	}
}

// stubSigner produces a deterministic 172-char base64-shaped hash. With
// AT_CERT_NUM=0 the fatcorews HashCharacters field is "0", so AT never
// validates these signatures in the uncertified test flow.
type stubSigner struct{}

func (stubSigner) Sign(canonical string) (string, string, error) {
	sum := sha256.Sum256([]byte(canonical))
	b64 := base64.StdEncoding.EncodeToString(sum[:])
	hash := strings.Repeat(b64, 4)[:172]
	return hash, "1", nil
}

// smokeProduct returns a valid domain.Product for smoke fixtures.
func smokeProduct() domain.Product {
	p, err := domain.NewProduct(domain.Product{
		ProductCode:        "SMOKE-001",
		ProductType:        domain.ProductTypeGoods,
		ProductDescription: "Produto Smoke",
		ProductNumberCode:  "SMOKE-001",
		Unit:               domain.UnitPiece,
		Active:             true,
	})
	if err != nil {
		panic(err)
	}
	return p
}

func one() domain.Quantity {
	q, err := domain.NewQuantity(1)
	if err != nil {
		panic(err)
	}
	return q
}

func mustEur(v float64) domain.Money {
	m, err := domain.NewMoney(v)
	if err != nil {
		panic(err)
	}
	return m
}

func movementVAT() domain.LineTax {
	rate, err := domain.GetTaxRate(domain.PT, domain.TaxNormal, "")
	if err != nil {
		panic(err)
	}
	return domain.VATTax{Rate: rate}
}

func runDocCommSmoke(ctx context.Context, client *at.Client, now time.Time) error {
	fmt.Println("→ document communication smoke (AT_TEST_COMM_ENABLED=1)")

	companyAddr, err := domain.NewAddress("Rua Teste 1", "Lisboa", "1000-001", "PT")
	if err != nil {
		return fmt.Errorf("company address: %w", err)
	}
	company := domain.Company{
		NIF:     domain.TaxID(os.Getenv("AT_NIF")),
		Name:    "Faturly Smoke LDA",
		Address: companyAddr,
		EACCode: "47190",
	}

	custAddr, err := domain.NewAddress("Av Dois 2", "Porto", "4000-002", "PT")
	if err != nil {
		return fmt.Errorf("customer address: %w", err)
	}
	customer, err := domain.NewCustomer(
		"SMOKE1",
		domain.CustomerTaxID("555555550"),
		"Cliente Smoke",
		custAddr,
		false,
	)
	if err != nil {
		return fmt.Errorf("customer: %w", err)
	}

	stamp := now.Format("0601021504")
	signer := stubSigner{}
	qrCfg := domain.QRConfig{IssuerNIF: company.NIF, CertificateNumber: "0"}

	var failures []string
	var registered []*domain.Series // every series that reaches RegisterWithAT successfully

	// --- FT: register series live, issue locally, communicate ---
	var issuedFT *domain.SalesInvoice // kept for NC leg below
	ftSeries, err := registerLiveSeries(ctx, client, "SMF"+stamp, domain.FT, false, now)
	if err != nil {
		failures = append(failures, fmt.Sprintf("FT registration: %v", err))
	} else {
		registered = append(registered, ftSeries)
		ftDraft, err := buildFTDraft(*customer, *ftSeries, now)
		if err != nil {
			failures = append(failures, fmt.Sprintf("FT draft: %v", err))
		} else {
			inv, err := domain.IssueSalesInvoice(ftDraft, ftSeries, signer, "smoke@faturly.pt", now, domain.IssueOptions{}, qrCfg)
			if err != nil {
				failures = append(failures, fmt.Sprintf("issue FT: %v", err))
			} else {
				fmt.Printf("  issued %s (ATCUD %s)\n", inv.Number.Format(), inv.ATCUD)
				invRes, err := client.CommunicateInvoice(ctx, company, inv)
				if err != nil {
					failures = append(failures, fmt.Sprintf("fatcorews: %v", err))
					fmt.Printf("  ✗ CommunicateInvoice: %v\n", err)
				} else {
					fmt.Printf("  fatcorews: code=%d %s\n", invRes.Code, invRes.Message)
					issuedFT = &inv
				}
			}
		}
	}

	// --- NC leg: credit note referencing the FT above ---
	if issuedFT != nil {
		ncSeries, err := registerLiveSeries(ctx, client, "SMN"+stamp, domain.NC, false, now)
		if err != nil {
			failures = append(failures, fmt.Sprintf("NC registration: %v", err))
		} else {
			registered = append(registered, ncSeries)
			ncDraft, err := buildNCDraft(*customer, *ncSeries, *issuedFT, now)
			if err != nil {
				failures = append(failures, fmt.Sprintf("NC draft: %v", err))
			} else {
				ncInv, err := domain.IssueSalesInvoice(ncDraft, ncSeries, signer, "smoke@faturly.pt", now, domain.IssueOptions{}, qrCfg)
				if err != nil {
					failures = append(failures, fmt.Sprintf("issue NC: %v", err))
				} else {
					fmt.Printf("  issued %s (ATCUD %s)\n", ncInv.Number.Format(), ncInv.ATCUD)
					ncRes, err := client.CommunicateInvoice(ctx, company, ncInv)
					if err != nil {
						failures = append(failures, fmt.Sprintf("fatcorews NC: %v", err))
						fmt.Printf("  ✗ fatcorews NC: %v\n", err)
					} else {
						fmt.Printf("  fatcorews NC: code=%d %s\n", ncRes.Code, ncRes.Message)
					}
				}
			}
		}
	}

	// --- GT: register series live, issue locally, communicate ---
	gtSeries, err := registerLiveSeries(ctx, client, "SMG"+stamp, domain.GT, false, now)
	if err != nil {
		failures = append(failures, fmt.Sprintf("GT registration: %v", err))
	} else {
		registered = append(registered, gtSeries)
		gtDraft, err := buildGTDraft(*customer, *gtSeries, now)
		if err != nil {
			failures = append(failures, fmt.Sprintf("GT draft: %v", err))
		} else {
			mv, err := domain.IssueStockMovement(gtDraft, gtSeries, signer, "smoke@faturly.pt", now, domain.IssueOptions{}, qrCfg)
			if err != nil {
				failures = append(failures, fmt.Sprintf("issue GT: %v", err))
			} else {
				fmt.Printf("  issued %s (ATCUD %s)\n", mv.Number.Format(), mv.ATCUD)
				mvRes, err := client.CommunicateTransport(ctx, company, mv)
				if err != nil {
					failures = append(failures, fmt.Sprintf("sgdtws: %v", err))
					fmt.Printf("  ✗ CommunicateTransport: %v\n", err)
				} else {
					fmt.Printf("  sgdtws: ATDocCodeID=%s\n", mvRes.ATDocCodeID)
				}
			}
		}
	}

	// --- Recovery-series probe: proves tipoSerie "R" live ---
	// No documents are issued into it; the cleanup loop below exercises anularSerie.
	fmt.Println("→ recovery-series probe (tipoSerie R)")
	rmrSeries, err := registerLiveSeries(ctx, client, "SMR"+stamp, domain.FT, true, now)
	if err != nil {
		failures = append(failures, fmt.Sprintf("recovery series: %v", err))
	} else {
		registered = append(registered, rmrSeries)
	}

	// --- cleanup: finalize or cancel every registered series ---
	for _, s := range registered {
		if s.LastNum > 0 {
			// Series has issued documents: finalize it.
			fin, err := at.FinalizationFor(*s, "smoke test cleanup")
			if err != nil {
				failures = append(failures, fmt.Sprintf("FinalizationFor %s: %v", s.ID, err))
				fmt.Printf("  ✗ FinalizationFor %s: %v\n", s.ID, err)
				continue
			}
			if err := client.FinalizeSeries(ctx, fin); err != nil {
				failures = append(failures, fmt.Sprintf("FinalizeSeries %s: %v", s.ID, err))
				fmt.Printf("  ✗ FinalizeSeries %s: %v\n", s.ID, err)
				continue
			}
			if err := s.Finalize(time.Now()); err != nil {
				failures = append(failures, fmt.Sprintf("domain Finalize %s: %v", s.ID, err))
				fmt.Printf("  ✗ domain Finalize %s: %v\n", s.ID, err)
				continue
			}
			fmt.Printf("  finalized series %s at lastSeq=%d\n", s.ID, s.LastNum)
		} else {
			// Series has no issued documents: cancel it instead.
			canc, err := at.CancellationFor(*s)
			if err != nil {
				failures = append(failures, fmt.Sprintf("CancellationFor %s: %v", s.ID, err))
				fmt.Printf("  ✗ CancellationFor %s: %v\n", s.ID, err)
				continue
			}
			if err := client.CancelSeries(ctx, canc); err != nil {
				failures = append(failures, fmt.Sprintf("CancelSeries %s: %v", s.ID, err))
				fmt.Printf("  ✗ CancelSeries %s: %v\n", s.ID, err)
				continue
			}
			if err := s.Cancel(time.Now()); err != nil {
				failures = append(failures, fmt.Sprintf("domain Cancel %s: %v", s.ID, err))
				fmt.Printf("  ✗ domain Cancel %s: %v\n", s.ID, err)
				continue
			}
			fmt.Printf("  cancelled series %s (no documents issued)\n", s.ID)
		}
	}

	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Printf("  FAIL: %s\n", f)
		}
		return fmt.Errorf("%d step(s) failed (series cleanup attempted)", len(failures))
	}
	return nil
}

func registerLiveSeries(ctx context.Context, client *at.Client, id string, dt domain.DocumentType, recovery bool, now time.Time) (*domain.Series, error) {
	var (
		s   domain.Series
		err error
	)
	if recovery {
		s, err = domain.NewRecoverySeries(id, dt)
	} else {
		s, err = domain.NewSeries(id, dt)
	}
	if err != nil {
		return nil, err
	}
	reg, err := at.RegistrationFor(s, now)
	if err != nil {
		return nil, err
	}
	res, err := client.RegisterSeries(ctx, reg)
	if err != nil {
		return nil, fmt.Errorf("register series %s: %w", id, err)
	}
	if err := s.RegisterWithAT(res.ValidationCode, res.RegistrationDate); err != nil {
		return nil, err
	}
	if recovery {
		fmt.Printf("  registered RECOVERY series %s (%s, tipoSerie R) code=%s\n", id, dt, res.ValidationCode)
	} else {
		fmt.Printf("  registered series %s (%s) code=%s\n", id, dt, res.ValidationCode)
	}
	return &s, nil
}

func buildFTDraft(customer domain.Customer, series domain.Series, now time.Time) (*domain.DraftSalesInvoice, error) {
	rate, err := domain.GetTaxRate(domain.PT, domain.TaxNormal, "")
	if err != nil {
		return nil, err
	}
	draft := &domain.DraftSalesInvoice{}
	draft.DocumentType = domain.FT
	draft.Customer = customer
	draft.Date = now
	draft.Series = series
	draft.AddLine(domain.DocumentLine{
		Product:      smokeProduct(),
		Quantity:     one(),
		UnitPrice:    mustEur(10),
		TaxPointDate: now,
		Tax:          domain.VATTax{Rate: rate},
	})
	return draft, nil
}

func buildNCDraft(customer domain.Customer, series domain.Series, orig domain.SalesInvoice, now time.Time) (*domain.DraftSalesInvoice, error) {
	line := domain.DocumentLine{
		Product:      smokeProduct(),
		Quantity:     one(),
		UnitPrice:    mustEur(10),
		TaxPointDate: now,
		Tax:          movementVAT(),
		References: []domain.DocReference{{
			Reference: orig.Number.Format(),
			Reason:    "Devolucao smoke test",
		}},
	}
	draft := &domain.DraftSalesInvoice{}
	draft.DocumentType = domain.NC
	draft.Customer = customer
	draft.Date = now
	draft.Series = series
	draft.AddLine(line)
	return draft, nil
}

func buildGTDraft(customer domain.Customer, series domain.Series, now time.Time) (*domain.DraftStockMovement, error) {
	shipFrom, err := domain.NewAddress("Armazem 1", "Lisboa", "1000-001", "PT")
	if err != nil {
		return nil, fmt.Errorf("ship_from address: %w", err)
	}
	shipTo, err := domain.NewAddress("Loja 9", "Porto", "4000-002", "PT")
	if err != nil {
		return nil, fmt.Errorf("ship_to address: %w", err)
	}
	draft := &domain.DraftStockMovement{}
	draft.DocumentType = domain.GT
	draft.Customer = customer
	draft.Date = now
	draft.Series = series
	draft.MovementStartTime = now.Add(2 * time.Hour)
	draft.ShipFrom = &domain.ShippingPoint{Address: &shipFrom}
	draft.ShipTo = &domain.ShippingPoint{Address: &shipTo}
	draft.AddLine(domain.DocumentLine{
		Product:      smokeProduct(),
		Quantity:     one(),
		UnitPrice:    mustEur(10),
		TaxPointDate: now,
		Tax:          movementVAT(),
	})
	return draft, nil
}
