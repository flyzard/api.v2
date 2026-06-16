# scripts

## saft-xsd-validate.sh — SAF-T (PT) XSD 1.1 gate

The bundled schema (`internal/adapter/saft/saftpt1.04_01.xsd`) is **XSD 1.1**
(`xs:assert`, unbounded `maxOccurs`); `xmllint` cannot compile it. Use an XSD 1.1
validator such as [xsd11-validator](https://github.com/dmaze/xsd11-validator)
(wraps Xerces-J) or Saxon-EE.

Run the matrix-wide gate:

    SAFT_XSD11_JAR=/path/to/xsd11-validator.jar \
    SAFT_XSD_CMD="bash $PWD/scripts/saft-xsd-validate.sh" \
      go test ./internal/adapter/saft -run XSD -v

Without `SAFT_XSD_CMD` the test skips (the default `go test ./...` does not
require Java). The `-v` run prints which matrix cases currently FAIL XSD — those
are the acceptance targets for Phase 3/5 (expected today: `taxbase_tax_only_line`).
