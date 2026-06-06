package qrimage

import (
	"bytes"
	"image"
	_ "image/png"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"
	zxqr "github.com/makiuchi-d/gozxing/qrcode"
)

// realisticPayload mirrors domain buildQRPayload output shape (qr_builder.go):
// "*"-separated key:value fields, ASCII. ~150 bytes — near the v9/v10 boundary
// region real documents live in.
const realisticPayload = "A:123456789*B:999999990*C:PT*D:FT*E:N*F:20260605*" +
	"G:FT A/1*H:ABCD1234-1*I1:PT*I7:100.00*I8:23.00*N:23.00*O:123.00*" +
	"Q:AbCd*R:0000"

// shortPayload would auto-fit far below version 9 — the exact case the
// previous AT certification trial failed on.
const shortPayload = "A:123456789*B:999999990"

// decodePNG decodes PNG bytes with gozxing (independent of the encoder) and
// returns the result.
func decodePNG(t *testing.T, png []byte) *gozxing.Result {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		t.Fatalf("image.Decode: %v", err)
	}
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		t.Fatalf("NewBinaryBitmapFromImage: %v", err)
	}
	res, err := zxqr.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		t.Fatalf("QR decode: %v", err)
	}
	return res
}

// Spec invariant 1: version floor. Short payload must be forced up to v9.
// Version is asserted structurally (gozxing does not expose it): v9 symbol is
// 53 modules + 4-module quiet zone each side = 61.
func TestShortPayloadForcedToVersion9(t *testing.T) {
	q, err := encode(shortPayload)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if q.VersionNumber != 9 {
		t.Errorf("VersionNumber = %d, want 9", q.VersionNumber)
	}
	if got := len(q.Bitmap()); got != 61 {
		t.Errorf("module grid edge = %d, want 61 (53 + 2*4 quiet zone)", got)
	}

	png, err := PNG(shortPayload, 256)
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	if got := decodePNG(t, png).GetText(); got != shortPayload {
		t.Errorf("decoded %q, want %q", got, shortPayload)
	}
}

// Payloads beyond v9 ECC-M byte capacity (180 bytes) legitimately encode at
// v10+ ("mínima" is a floor, not an exact version).
func TestLongPayloadExceedsVersion9(t *testing.T) {
	long := realisticPayload + "*S:" + strings.Repeat("X", 200)
	q, err := encode(long)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if q.VersionNumber <= 9 {
		t.Errorf("VersionNumber = %d, want > 9", q.VersionNumber)
	}
	if got := len(q.Bitmap()); got <= 61 {
		t.Errorf("module grid edge = %d, want > 61", got)
	}

	png, err := PNG(long, 256)
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	if got := decodePNG(t, png).GetText(); got != long {
		t.Errorf("decoded %q, want %q", got, long)
	}
}

// Spec invariant 2: ECC exactly M, verified via independent decode metadata.
func TestRealisticPayloadRoundTripECCM(t *testing.T) {
	png, err := PNG(realisticPayload, 256)
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	res := decodePNG(t, png)
	if got := res.GetText(); got != realisticPayload {
		t.Errorf("decoded %q, want %q", got, realisticPayload)
	}
	ec := res.GetResultMetadata()[gozxing.ResultMetadataType_ERROR_CORRECTION_LEVEL]
	if ec != "M" {
		t.Errorf("ECC level = %v, want M", ec)
	}
}

// skip2 silently returns a larger image when sizePx is smaller than the
// symbol needs (part of PNG's documented contract). Pin it: 1 px requested,
// v9 symbol → 61×61 px (1 px/module incl quiet zone), still decodable.
// Guards against a dependency bump ever downscaling (= module corruption)
// instead of enlarging.
func TestUndersizedRequestReturnsLargerImage(t *testing.T) {
	png, err := PNG(shortPayload, 1)
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		t.Fatalf("image.Decode: %v", err)
	}
	if w := img.Bounds().Dx(); w != 61 {
		t.Errorf("image edge = %d px, want 61 (v9 modules at 1 px/module)", w)
	}
	if got := decodePNG(t, png).GetText(); got != shortPayload {
		t.Errorf("decoded %q, want %q", got, shortPayload)
	}
}

func TestEmptyPayloadErrors(t *testing.T) {
	if _, err := PNG("", 256); err == nil {
		t.Error("want error for empty payload, got nil")
	}
}

func TestNonPositiveSizeErrors(t *testing.T) {
	for _, size := range []int{0, -1, -256} {
		if _, err := PNG(realisticPayload, size); err == nil {
			t.Errorf("want error for sizePx=%d, got nil", size)
		}
	}
}
