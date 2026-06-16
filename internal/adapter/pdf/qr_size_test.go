package pdf

import "testing"

// TestQRRasterFloor guards the QR raster resolution against silent shrinkage.
// qrSizePx is the PNG edge in pixels; at ~300 dpi the 30×30 mm Despacho 195/2020
// minimum is 30/25.4*300 ≈ 354 px. A prior AT trial was rejected over QR sizing,
// so this floor must never regress. (Full print-mm guard lands in Phase 4.)
func TestQRRasterFloor(t *testing.T) {
	const minPxFor30mmAt300dpi = 354 // 30 mm / 25.4 mm-per-inch * 300 dpi
	if qrSizePx < minPxFor30mmAt300dpi {
		t.Errorf("qrSizePx = %d, below the 30 mm @ 300 dpi raster floor (%d px)", qrSizePx, minPxFor30mmAt300dpi)
	}
}
