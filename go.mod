module github.com/flyzard/invoicing.v2

go 1.26.1

require github.com/google/uuid v1.6.0

// C3: fork forces a single ISO/IEC 18004 Byte-mode segment (AT compliance).
replace github.com/skip2/go-qrcode => ./third_party/go-qrcode

require golang.org/x/text v0.37.0

require (
	github.com/johnfercher/maroto/v2 v2.4.0
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	golang.org/x/time v0.15.0
)

require (
	github.com/boombuler/barcode v1.1.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/f-amaral/go-async v0.3.0 // indirect
	github.com/hhrutter/lzw v1.0.0 // indirect
	github.com/hhrutter/pkcs7 v0.2.0 // indirect
	github.com/hhrutter/tiff v1.0.2 // indirect
	github.com/johnfercher/go-tree v1.1.0 // indirect
	github.com/mattn/go-runewidth v0.0.21 // indirect
	github.com/pdfcpu/pdfcpu v0.11.1 // indirect
	github.com/phpdave11/gofpdf v1.4.3 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/image v0.37.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
