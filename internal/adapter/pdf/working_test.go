package pdf

import (
	"errors"
	"testing"

	"github.com/johnfercher/maroto/v2/pkg/test"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

func fixtureWorkDoc(t *testing.T) domain.WorkDocument {
	t.Helper()
	ft := fixtureFT(t)
	wd := domain.WorkDocument{IssuedDocument: ft.IssuedDocument}
	wd.Number = domain.DocNumber{Type: domain.PF, Series: "A2026", Seq: 5}
	wd.DocumentType = domain.PF
	return wd
}

func TestBuildWorkDocument_Structure(t *testing.T) {
	eng, err := buildWorkDocument(fixtureWorkDoc(t), validMeta(), false)
	if err != nil {
		t.Fatal(err)
	}
	test.New(t).Assert(eng.GetStructure()).Equals("pf_basic.json")
}

func TestRenderWorkDocument_MissingQR(t *testing.T) {
	wd := fixtureWorkDoc(t)
	wd.QRPayload = ""
	if _, err := RenderWorkDocument(wd, validMeta()); !errors.Is(err, ErrMissingQRPayload) {
		t.Fatalf("want ErrMissingQRPayload, got %v", err)
	}
}
