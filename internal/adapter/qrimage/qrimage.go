// Package qrimage rasterizes frozen domain QR payload strings
// (IssuedDocument.QRPayload) into PNG images.
//
// It enforces "Especificações Técnicas Código de Barras Bidimensional —
// Código QR" n.º 2 unconditionally: symbol version >= 9 and error-correction
// level exactly M. A previous AT certification trial was rejected for a
// version below 9 (QR libraries auto-pick the smallest version that fits, so
// short documents under-shoot the floor); the invariants are therefore not
// configurable. The 4-module quiet zone (ISO/IEC 18004) is kept via skip2's
// default border.
//
// Payload composition lives in domain (qr_builder.go); this package is the
// Tier-3 image concern that consumes the frozen string at print time.
package qrimage

import (
	"errors"

	"github.com/skip2/go-qrcode"
)

// minVersion is the AT-mandated QR symbol version floor (spec n.º 2).
const minVersion = 9

// encode builds the QR symbol at ECC M, forcing the version up to minVersion
// when the auto-selected version under-shoots it. Forcing only ever increases
// capacity, so the forced call cannot fail on content length.
func encode(payload string) (*qrcode.QRCode, error) {
	q, err := qrcode.New(payload, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	if q.VersionNumber < minVersion {
		q, err = qrcode.NewWithForcedVersion(payload, minVersion, qrcode.Medium)
		if err != nil {
			return nil, err
		}
	}
	return q, nil
}

// PNG renders payload as a PNG image of edge sizePx pixels.
//
// If sizePx is smaller than the symbol needs, a larger image is returned
// (skip2 behavior) — module integrity never degrades, the caller gets
// at-least-sizePx. The 30×30 mm minimum print size is the caller's print-time
// concern.
func PNG(payload string, sizePx int) ([]byte, error) {
	if payload == "" {
		return nil, errors.New("qrimage: empty payload")
	}
	if sizePx <= 0 {
		return nil, errors.New("qrimage: sizePx must be positive")
	}
	q, err := encode(payload)
	if err != nil {
		return nil, err
	}
	return q.PNG(sizePx)
}
