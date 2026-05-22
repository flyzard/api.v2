package main

import (
	"fmt"
	"log"
	"time"

	"github.com/flyzard/invoicing.v2/domain"
)

// fixtures is the shared cast of objects every scenario draws from.
// One Company, one User, a handful of Customers, a Product catalogue,
// and one registered Series per DocumentType used in the demo.
type fixtures struct {
	Software   domain.SoftwareIdentity
	Issuer     domain.Company
	IssuerUser domain.User

	CustWithNIF domain.Customer // identified + NIF (5.1, 5.2, 5.3-5.6, 5.7, 5.13)
	CustNoNIF1  domain.Customer // identified but no NIF (5.9)
	CustNoNIF2  domain.Customer // another identified, no NIF (5.10)
	CustForeign domain.Customer // US customer for foreign-currency invoice (5.8)

	Products map[string]domain.Product
	Series   map[domain.DocumentType]*domain.Series
}

func buildFixtures(now time.Time) *fixtures {
	f := &fixtures{}

	f.Software = must(domain.NewSoftwareIdentity(domain.SoftwareIdentity{
		ProducerTaxID:     "519348761",
		SoftwareName:      "Faturly",
		ProducerName:      "AVENIDA DO CODIGO LDA",
		Version:           "2.0.0",
		CertificateNumber: "9999",
	}))

	issuerAddr := must(domain.NewAddress("Rua dos Programadores 1", "Lisboa", "1000-100", "PT"))
	f.Issuer = must(domain.NewCompany(domain.Company{
		NIF:        "500000000",
		Name:       "Demo Faturação Lda.",
		TradeName:  "Demo Faturação",
		Address:    issuerAddr,
		FiscalYear: 2026,
		StartMonth: 1,
		EACCode:    "47190", // retail (47xxx) → enables FS €1000 retail tier
		Active:     true,
	}))

	f.IssuerUser = must(domain.NewUser("issuer@demo.pt", "Maria Operadora"))

	f.CustWithNIF = *must(domain.NewCustomer(
		"ACC-PT-001", "503504564", "Acme Lda.",
		must(domain.NewAddress("Rua das Flores 12", "Lisboa", "1000-001", "PT")),
		false,
	))

	f.CustNoNIF1 = *must(domain.NewCustomer(
		"ACC-PT-NONIF-1", "999999990", "Joana Silva",
		must(domain.NewAddress("Av. da República 50", "Porto", "4000-200", "PT")),
		false,
	))

	f.CustNoNIF2 = *must(domain.NewCustomer(
		"ACC-PT-NONIF-2", "999999990", "Pedro Costa",
		must(domain.NewAddress("Rua de Santa Catarina 33", "Porto", "4000-300", "PT")),
		false,
	))

	f.CustForeign = *must(domain.NewCustomer(
		"ACC-US-001", "EIN-12-3456789", "Globex Corp.",
		must(domain.NewAddress("742 Evergreen Terrace", "Springfield", "12345", "US")),
		false,
	))

	f.Products = map[string]domain.Product{
		"P-RED":     mustProduct("P-RED", domain.ProductTypeGoods, "Pão de mistura 500g", domain.UnitKg),
		"P-INT":     mustProduct("P-INT", domain.ProductTypeGoods, "Conserva de atum 120g", domain.UnitPiece),
		"P-NOR":     mustProduct("P-NOR", domain.ProductTypeGoods, "Auriculares Bluetooth", domain.UnitPiece),
		"P-EXEMPT":  mustProduct("P-EXEMPT", domain.ProductTypeService, "Consulta médica geral", domain.UnitService),
		"P-SERVICE": mustProduct("P-SERVICE", domain.ProductTypeService, "Hora de consultoria técnica", domain.UnitHour),
		"P-CRATE":   mustProduct("P-CRATE", domain.ProductTypeGoods, "Caixa de transporte 60x40", domain.UnitPiece),
	}

	// One series per used DocumentType. AT validation codes are placeholders
	// satisfying the permissive [A-Z0-9]{>=8} rule in domain.ValidateATCode.
	f.Series = map[domain.DocumentType]*domain.Series{}
	doctypes := []domain.DocumentType{
		domain.FT, domain.FS, domain.FR, domain.NC, domain.ND,
		domain.GR, domain.GT, domain.GA, domain.GC, domain.GD,
		domain.OR, domain.PF, domain.NE, domain.CM, domain.FC, domain.FO,
		domain.RC, domain.RG,
	}
	for i, dt := range doctypes {
		seriesID := string(dt) + "2026"
		atCode := fmt.Sprintf("ATCODE%02d", i+1)
		f.Series[dt] = makeSeries(seriesID, dt, atCode, now)
	}

	return f
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

