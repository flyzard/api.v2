// Fixtures and the shared cast every §5 scenario draws from.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// catItem is one catalogue row: the product snapshot plus its default net unit
// price and default line tax. Scenarios that deviate (line discount, foreign
// currency, non-subject movement) override at the call site.
type catItem struct {
	p     domain.Product
	price float64
	tax   func() domain.LineTax
}

// Fixtures is the shared cast every scenario draws from: one Company + User,
// the six reviewed customers (keyed by their C0xx id, plus "SELF" for own-asset
// movements), the product catalogue, and one registered Series per DocumentType.
type Fixtures struct {
	Issuer     domain.Company
	IssuerUser domain.User

	Cust   map[string]domain.Customer
	Cat    map[string]catItem
	Series map[domain.DocumentType]*domain.Series
}

func BuildFixtures(now time.Time) *Fixtures {
	f := &Fixtures{}

	issuerAddr := must(domain.NewAddress("Travessa Serradinha, 46, 1 ESQ. A", "BENEDITA", "2475-116", "PT"))
	f.Issuer = must(domain.NewCompany(domain.Company{
		NIF:        "519348761",
		Name:       "AVENIDA DO CODIGO - SOFTWARE E SOLUÇÕES DIGITAIS LDA",
		TradeName:  "Faturly",
		Address:    issuerAddr,
		FiscalYear: 2026,
		StartMonth: 1,
		EACCode:    "47190", // retail (47xxx) → enables FS €1000 retail tier
		Active:     true,
	}))

	f.IssuerUser = must(domain.NewUser("issuer@demo.pt", "Maria Operadora"))

	f.Cust = map[string]domain.Customer{
		"C001": cust("248031562", "Maria da Conceição Silva", "Rua das Flores, 45, 2.º Esq", "Lisboa", "1200-194", "PT"),
		"C002": cust("502819472", "Restaurante O Cantinho, Lda", "Av. da Boavista, 1200", "Porto", "4100-130", "PT"),
		"C003": cust("517603144", "Mercearia Central, Unipessoal Lda", "Rua Ferreira Borges, 88", "Coimbra", "3000-179", "PT"),
		"C004": cust(string(domain.FinalConsumerNIF), "João Pedro Martins", "Rua do Sol, 12", "Caldas da Rainha", "2500-100", "PT"),
		"C005": cust(string(domain.FinalConsumerNIF), "Ana Rita Ferreira", "Travessa da Igreja, 3", "Alcobaça", "2460-050", "PT"),
		"C006": cust("US-EIN-47-1822910", "Atlantic Beverages LLC", "350 5th Avenue, Suite 4100", "New York", "NY 10118", "US"),
		// SELF stands in as the consignee for GA (own-asset movement, no external
		// customer): transport docs still require a valid customer.
		"SELF": cust(string(f.Issuer.NIF), f.Issuer.Name, issuerAddr.AddressDetail, issuerAddr.City, issuerAddr.PostalCode, "PT"),
	}

	f.Cat = map[string]catItem{
		"P001": ci("P001", domain.ProductTypeGoods, "Pão de Forma Integral 500g", domain.UnitPiece, 1.29, taxRED),
		"P002": ci("P002", domain.ProductTypeService, "Serviço de Formação Profissional (módulo)", domain.UnitPiece, 150.00,
			func() domain.LineTax { return taxEXEMPT(domain.M07, "Isento artigo 9.º do CIVA") }),
		"P003": ci("P003", domain.ProductTypeGoods, "Vinho Tinto Douro DOC 75cl", domain.UnitPiece, 8.90, taxINT),
		"P004": ci("P004", domain.ProductTypeGoods, "Gin Premium 70cl", domain.UnitPiece, 24.50, taxNOR),
		"P005": ci("P005", domain.ProductTypeGoods, "Saco Reutilizável Eco", domain.UnitPiece, 0.55, taxNOR),
		"P006": ci("P006", domain.ProductTypeGoods, "Cerveja Artesanal IPA 33cl", domain.UnitPiece, 1.80, taxNOR),
		"P007": ci("P007", domain.ProductTypeGoods, "Café Torrado Moído 250g", domain.UnitPiece, 3.45, taxNOR),
		"P008": ci("P008", domain.ProductTypeGoods, "Rebuçado Mentol (unidade)", domain.UnitPiece, 0.05, taxNOR),
		"P010": ci("P010", domain.ProductTypeService, "Serviço de Consultoria (hora)", domain.UnitHour, 75.00, taxNOR),
		"P011": ci("P011", domain.ProductTypeGoods, "Caixa de Vinho Douro (6 garrafas)", domain.UnitPiece, 53.40, taxINT),
		// P012 default is NS (own-asset movement, not a transmission); used by GA via c.line.
		"P012": ci("P012", domain.ProductTypeOther, "Arca Refrigeradora (ativo próprio)", domain.UnitPiece, 1850.00,
			func() domain.LineTax { return nsTax(domain.M99, "Movimentação de ativo próprio") }),
		"P013": ci("P013", domain.ProductTypeService, "Mão de Obra Técnica (hora)", domain.UnitHour, 35.00, taxNOR),
	}

	// One series per used DocumentType. AT validation codes are placeholders
	// satisfying the permissive [A-Z0-9]{>=8} rule in domain.ValidateATCode.
	f.Series = map[domain.DocumentType]*domain.Series{}
	doctypes := []domain.DocumentType{
		domain.FT, domain.FS, domain.FR, domain.NC, domain.ND,
		domain.GR, domain.GT, domain.GA, domain.GC, domain.GD,
		domain.OR, domain.PF, domain.NE, domain.CM, domain.FC, domain.FO, domain.OU,
		domain.RC, domain.RG,
	}
	for i, dt := range doctypes {
		seriesID := string(dt) + "2026"
		atCode := fmt.Sprintf("ATCODE%02d", i+1)
		f.Series[dt] = makeSeries(seriesID, dt, atCode, now)
	}

	return f
}

func cust(taxID, name, detail, city, zip string, country domain.Country) domain.Customer {
	addr := must(domain.NewAddress(detail, city, zip, country))
	return *must(domain.NewCustomer(domain.CustomerTaxID(taxID), name, addr, false))
}

func ci(code string, kind domain.ProductType, desc string, unit domain.UnitOfMeasure, price float64, tax func() domain.LineTax) catItem {
	return catItem{p: mustProduct(code, kind, desc, unit), price: price, tax: tax}
}

// nsTax builds a "não sujeito" (NS) line tax — valued line, zero VAT — for the
// own-asset / consignment / return movements (GA/GC/GD).
func nsTax(reason domain.Exemption, text string) domain.LineTax {
	return must(domain.NewNotSubjectLineTax(domain.TaxJurisdiction("PT"), reason, text))
}

func mustProduct(code string, kind domain.ProductType, desc string, unit domain.UnitOfMeasure) domain.Product {
	return must(domain.NewProduct(domain.Product{
		ProductCode:        code,
		ProductType:        kind,
		ProductDescription: desc,
		ProductNumberCode:  code,
		Unit:               unit,
		Active:             true,
	}))
}

func makeSeries(id string, dt domain.DocumentType, atCode string, at time.Time) *domain.Series {
	s, err := domain.NewSeries(id, dt)
	if err != nil {
		log.Fatalf("new series %s: %v", id, err)
	}
	if err := s.RegisterWithAT(atCode, at); err != nil {
		log.Fatalf("register series %s: %v", id, err)
	}
	return &s
}
