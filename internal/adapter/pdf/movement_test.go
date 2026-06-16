package pdf

import (
	"testing"
	"time"

	"github.com/johnfercher/maroto/v2/pkg/test"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

func fixtureGT(t *testing.T) domain.StockMovement {
	t.Helper()
	ft := fixtureFT(t)
	sm := domain.StockMovement{IssuedDocument: ft.IssuedDocument}
	sm.Number = domain.DocNumber{Type: domain.GT, Series: "A2026", Seq: 11}
	sm.DocumentType = domain.GT
	sm.MovementStartTime = time.Date(2026, 5, 10, 8, 30, 0, 0, time.UTC)
	sm.ATDocCodeID = "ATCODE12345"
	sm.ShipFrom = &domain.ShippingPoint{Address: &domain.Address{
		AddressDetail: "Armazém Central, Rua A 1", City: "Lisboa", PostalCode: "1000-001", Country: "PT",
	}}
	sm.ShipTo = &domain.ShippingPoint{Address: &domain.Address{
		AddressDetail: "Loja Norte, Rua B 2", City: "Braga", PostalCode: "4700-001", Country: "PT",
	}}
	return sm
}

func TestBuildStockMovement_Structure(t *testing.T) {
	eng, err := buildStockMovement(fixtureGT(t), validMeta(), false)
	if err != nil {
		t.Fatal(err)
	}
	test.New(t).Assert(eng.GetStructure()).Equals("gt_basic.json")
}

func fixtureGTThirdParty(t *testing.T) domain.StockMovement {
	t.Helper()
	sm := fixtureGT(t)
	sm.Status = domain.StatusThirdParty
	return sm
}

func TestBuildStockMovement_ThirdParty(t *testing.T) {
	eng, err := buildStockMovement(fixtureGTThirdParty(t), validMeta(), false)
	if err != nil {
		t.Fatal(err)
	}
	test.New(t).Assert(eng.GetStructure()).Equals("gt_third_party.json")
}
