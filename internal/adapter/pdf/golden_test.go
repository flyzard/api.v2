package pdf

import (
	"os"
	"testing"

	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/test"
)

// TestUpdateGoldens rewrites every golden structure JSON. Run after a
// deliberate layout change:
//
//	UPDATE_GOLDEN=1 go test ./internal/adapter/pdf -run TestUpdateGoldens
func TestUpdateGoldens(t *testing.T) {
	if os.Getenv("UPDATE_GOLDEN") == "" {
		t.Skip("set UPDATE_GOLDEN=1 to rewrite golden files")
	}
	save := func(file string, eng core.Maroto, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		test.New(t).Assert(eng.GetStructure()).Save(file)
	}

	ft, e := buildSalesInvoice(fixtureFT(t), validMeta(), false)
	save("ft_basic.json", ft, e)
	nc, e := buildSalesInvoice(fixtureNC(t), validMeta(), false)
	save("nc_references.json", nc, e)
	fc, e := buildSalesInvoice(fixtureFTCancelled(t), validMeta(), false)
	save("ft_cancelled.json", fc, e)
	fw, e := buildSalesInvoice(fixtureFTWithholding(t), validMeta(), false)
	save("ft_withholding.json", fw, e)
	fr, e := buildSalesInvoice(fixtureFR(t), validMeta(), false)
	save("fr_payments.json", fr, e)
	fs, e := buildSalesInvoice(fixtureFSAnonymous(t), validMeta(), false)
	save("fs_anonymous.json", fs, e)
	ex, e := buildSalesInvoice(fixtureFTExempt(t), validMeta(), false)
	save("vat_exempt.json", ex, e)
	gt, e := buildStockMovement(fixtureGT(t), validMeta(), false)
	save("gt_basic.json", gt, e)
	gtp, e := buildStockMovement(fixtureGTThirdParty(t), validMeta(), false)
	save("gt_third_party.json", gtp, e)
	pf, e := buildWorkDocument(fixtureWorkDoc(t), validMeta(), false)
	save("pf_basic.json", pf, e)
	rg, e := buildPayment(fixtureRG(t), validMeta())
	save("rg_basic.json", rg, e)
	rw, e := buildPayment(fixtureRGWithholding(t), validMeta())
	save("rg_withholding.json", rw, e)
}
