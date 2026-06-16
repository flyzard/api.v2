package saft

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestExport_XSDValidates shells out to an external XSD 1.1 validator when
// SAFT_XSD_CMD is set (the exported file path is appended as the last arg), e.g.:
//
//	SAFT_XSD11_JAR=/opt/xsd11-validator.jar \
//	SAFT_XSD_CMD="bash $PWD/scripts/saft-xsd-validate.sh" \
//	  go test ./internal/adapter/saft -run XSD -v
//
// The command must resolve independent of CWD: `go test` runs in the package dir, so use an absolute path (e.g. $PWD from the repo root).
//
// It validates EVERY matrix case (matrix_test.go) and reports each failure with
// t.Errorf (not Fatal) so the run prints the full baseline of currently-invalid
// cases — those are the Phase 3/5 acceptance targets.
func TestExport_XSDValidates(t *testing.T) {
	validator := os.Getenv("SAFT_XSD_CMD")
	if validator == "" {
		t.Skip("SAFT_XSD_CMD not set; XSD validation runs out-of-band (see scripts/README.md)")
	}
	parts := strings.Fields(validator)

	for _, c := range matrixDocs() {
		t.Run(c.name, func(t *testing.T) {
			out := mustExport(t, c.sales, c.stock, c.work, c.payments)
			path := filepath.Join(t.TempDir(), "audit.xml")
			if err := os.WriteFile(path, out, 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(parts[0], append(parts[1:], path)...)
			if outb, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("XSD validation failed: %v\n%s", err, outb)
			}
		})
	}
}
