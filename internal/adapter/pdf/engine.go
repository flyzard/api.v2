package pdf

import (
	_ "embed"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
	"github.com/johnfercher/maroto/v2/pkg/repository"
)

//go:embed fonts/LiberationSans-Regular.ttf
var fontRegular []byte

//go:embed fonts/LiberationSans-Bold.ttf
var fontBold []byte

// fontFamily is the embedded UTF-8 font (Portuguese diacritics).
const fontFamily = "liberation"

// newEngine builds a configured A4 maroto instance: embedded fonts, 10 mm
// margins (maroto defaults), automatic "Página n de N" footer.
func newEngine() (core.Maroto, error) {
	fonts, err := repository.New().
		AddUTF8FontFromBytes(fontFamily, fontstyle.Normal, fontRegular).
		AddUTF8FontFromBytes(fontFamily, fontstyle.Bold, fontBold).
		Load()
	if err != nil {
		return nil, err
	}
	cfg := config.NewBuilder().
		WithCustomFonts(fonts).
		WithDefaultFont(&props.Font{Family: fontFamily, Size: 9}).
		WithPageNumber(props.PageNumber{
			Pattern: "Página {current} de {total}",
			Place:   props.RightBottom,
			Size:    7,
		}).
		Build()
	return maroto.New(cfg), nil
}

// render turns a build* result into PDF bytes; shared tail of every Render*.
func render(eng core.Maroto, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	doc, err := eng.Generate()
	if err != nil {
		return nil, err
	}
	return doc.GetBytes(), nil
}
