#!/usr/bin/env bash
# Validates a SAF-T (PT) XML file against the bundled XSD 1.1 schema.
#
# Usage (wired into xsd_test.go via SAFT_XSD_CMD, which appends the file path):
#   SAFT_XSD11_JAR=/opt/xsd11-validator.jar \
#   SAFT_XSD_CMD="bash $PWD/scripts/saft-xsd-validate.sh" \
#     go test ./internal/adapter/saft -run XSD -v
#
# xmllint CANNOT be used: the schema is XSD 1.1 (xs:assert + unbounded maxOccurs).
# Provide an XSD 1.1 validator via SAFT_XSD11_JAR (Xerces-J / xsd11-validator jar).
set -euo pipefail
file="${1:?usage: saft-xsd-validate.sh <file.xml>}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
schema="$root/internal/adapter/saft/saftpt1.04_01.xsd"

if [[ -n "${SAFT_XSD11_JAR:-}" ]]; then
  exec java -jar "$SAFT_XSD11_JAR" -sf "$schema" -if "$file"
fi
echo "no XSD 1.1 validator configured: set SAFT_XSD11_JAR to a Xerces-J / xsd11-validator jar" >&2
exit 2
