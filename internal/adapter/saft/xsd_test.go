package saft

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestExport_XSDValidates shells out to an external XSD 1.1 validator when
// SAFT_XSD_CMD is set, e.g.:
//
//	SAFT_XSD_CMD="java -jar /opt/xsd11-validator.jar -sf SAFTPT_1_04_01.xsd -if" \
//	  go test ./internal/adapter/saft -run XSDValidates -v
//
// The exported file path is appended as the last argument.
func TestExport_XSDValidates(t *testing.T) {
	validator := os.Getenv("SAFT_XSD_CMD")
	if validator == "" {
		t.Skip("SAFT_XSD_CMD not set; XSD validation runs out-of-band")
	}

	hdr := gdTestHeader()
	hdr.Issuer.EACCode = "47190"
	sales, stock, work, payments := goldenDocs()
	out, err := Export(hdr, sales, stock, work, payments)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "audit.xml")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	parts := strings.Fields(validator)
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	if outb, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("XSD validation failed: %v\n%s", err, outb)
	}
}
