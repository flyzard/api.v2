// Fixtures and the shared cast every §5 scenario draws from.
package main

import (
	"fmt"

	"github.com/flyzard/invoicing.v2/internal/app"
)

// finalConsumerNIF is AT's "consumidor final" sentinel. C004/C005 use it but stay
// distinct and non-anonymous: customerFrom keys final-consumer customers on
// name+address, so two named final consumers map to two CustomerIDs.
const finalConsumerNIF = "999999990"

// catItem is one catalogue row: the product snapshot plus its default net unit
// price (in CENTS) and default line tax. Scenarios that deviate (line discount,
// foreign currency, non-subject movement) override at the call site.
type catItem struct {
	code, ptype, desc, unit string
	priceCents              int64
	tax                     func() *app.LineTaxInput
}

// issuerIdentity is the producer company's primitives. main.go builds the
// domain.Company (composition root); scenarios consume the app.UserInput.
type issuerIdentity struct {
	NIF, Name, TradeName        string
	Detail, City, PostalCode    string
	FiscalYear, StartMonth, EAC int
}

// seriesSpec is one (id, docType, atCode) tuple main.go seeds via
// SeedRegisteredSeries.
type seriesSpec struct {
	id, docType, atCode string
}

// Fixtures is the shared cast every scenario draws from: one issuer + user, the
// six reviewed customers (keyed by their C0xx id, plus "SELF" for own-asset
// movements), the product catalogue, and one registered Series per DocumentType.
type Fixtures struct {
	Issuer     issuerIdentity
	IssuerUser app.UserInput

	Cust   map[string]app.CustomerInput
	Cat    map[string]catItem
	Series []seriesSpec
}

func BuildFixtures() *Fixtures {
	f := &Fixtures{}

	f.Issuer = issuerIdentity{
		NIF:        "519348761",
		Name:       "AVENIDA DO CODIGO - SOFTWARE E SOLUÇÕES DIGITAIS LDA",
		TradeName:  "Faturly",
		Detail:     "Travessa Serradinha, 46, 1 ESQ. A",
		City:       "BENEDITA",
		PostalCode: "2475-116",
		FiscalYear: 2026,
		StartMonth: 1,
		EAC:        47190, // retail (47xxx) → enables FS €1000 retail tier
	}

	f.IssuerUser = app.UserInput{Email: "issuer@demo.pt", Name: "Maria Operadora"}

	f.Cust = map[string]app.CustomerInput{
		"C001": cust("248031562", "Maria da Conceição Silva", "Rua das Flores, 45, 2.º Esq", "Lisboa", "1200-194", "PT"),
		"C002": cust("502819472", "Restaurante O Cantinho, Lda", "Av. da Boavista, 1200", "Porto", "4100-130", "PT"),
		"C003": cust("517603144", "Mercearia Central, Unipessoal Lda", "Rua Ferreira Borges, 88", "Coimbra", "3000-179", "PT"),
		"C004": cust(finalConsumerNIF, "João Pedro Martins", "Rua do Sol, 12", "Caldas da Rainha", "2500-100", "PT"),
		"C005": cust(finalConsumerNIF, "Ana Rita Ferreira", "Travessa da Igreja, 3", "Alcobaça", "2460-050", "PT"),
		"C006": cust("US-EIN-47-1822910", "Atlantic Beverages LLC", "350 5th Avenue, Suite 4100", "New York", "NY 10118", "US"),
		// SELF stands in as the consignee for GA (own-asset movement, no external
		// customer): transport docs still require a valid customer.
		"SELF": cust(f.Issuer.NIF, f.Issuer.Name, f.Issuer.Detail, f.Issuer.City, f.Issuer.PostalCode, "PT"),
	}

	f.Cat = map[string]catItem{
		"P001": ci("P001", app.ProductGoods, "Pão de Forma Integral 500g", app.UnitPiece, 129, taxRED),
		"P002": ci("P002", app.ProductService, "Serviço de Formação Profissional (módulo)", app.UnitPiece, 15000,
			func() *app.LineTaxInput { return taxEXEMPT(app.ExemptM07, "Isento artigo 9.º do CIVA") }),
		"P003": ci("P003", app.ProductGoods, "Vinho Tinto Douro DOC 75cl", app.UnitPiece, 890, taxINT),
		"P004": ci("P004", app.ProductGoods, "Gin Premium 70cl", app.UnitPiece, 2450, taxNOR),
		"P005": ci("P005", app.ProductGoods, "Saco Reutilizável Eco", app.UnitPiece, 55, taxNOR),
		"P006": ci("P006", app.ProductGoods, "Cerveja Artesanal IPA 33cl", app.UnitPiece, 180, taxNOR),
		"P007": ci("P007", app.ProductGoods, "Café Torrado Moído 250g", app.UnitPiece, 345, taxNOR),
		"P008": ci("P008", app.ProductGoods, "Rebuçado Mentol (unidade)", app.UnitPiece, 5, taxNOR),
		"P010": ci("P010", app.ProductService, "Serviço de Consultoria (hora)", app.UnitHour, 7500, taxNOR),
		"P011": ci("P011", app.ProductGoods, "Caixa de Vinho Douro (6 garrafas)", app.UnitPiece, 5340, taxINT),
		// P012 default is NS (own-asset movement, not a transmission); used by GA via c.line.
		"P012": ci("P012", app.ProductOther, "Arca Refrigeradora (ativo próprio)", app.UnitPiece, 185000,
			func() *app.LineTaxInput { return nsTax(app.ExemptM99, "Movimentação de ativo próprio") }),
		"P013": ci("P013", app.ProductService, "Mão de Obra Técnica (hora)", app.UnitHour, 3500, taxNOR),
	}

	// One series per used DocumentType. AT validation codes are placeholders
	// satisfying the permissive [A-Z0-9]{>=8} rule in domain.ValidateATCode.
	doctypes := []string{
		app.DocFT, app.DocFS, app.DocFR, app.DocNC, app.DocND,
		app.DocGR, app.DocGT, app.DocGA, app.DocGC, app.DocGD,
		app.DocOR, app.DocPF, app.DocNE, app.DocCM, app.DocFC, app.DocFO, app.DocOU,
		app.DocRC, app.DocRG,
	}
	for i, dt := range doctypes {
		f.Series = append(f.Series, seriesSpec{
			id:      dt + "2026",
			docType: dt,
			atCode:  fmt.Sprintf("ATCODE%02d", i+1),
		})
	}

	return f
}

func cust(taxID, name, detail, city, zip, country string) app.CustomerInput {
	return app.CustomerInput{
		TaxID:   taxID,
		Name:    name,
		Country: country,
		Address: &app.AddressInput{Detail: detail, City: city, PostalCode: zip},
	}
}

func ci(code, ptype, desc, unit string, priceCents int64, tax func() *app.LineTaxInput) catItem {
	return catItem{code: code, ptype: ptype, desc: desc, unit: unit, priceCents: priceCents, tax: tax}
}
