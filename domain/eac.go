package domain

// retailMotorEACs lists the CAE-Rev.3 codes that count as motor-vehicle retail
// activity for VAT-regime purposes. Source: AT instruction on EAC retail mapping.
var retailMotorEACs = map[string]bool{
	"45110": true,
	"45190": true,
	"45320": true,
	"45401": true,
	"45402": true,
}

func IsRetailActivity(eacCode string) bool {
	if len(eacCode) != 5 {
		return false
	}
	if eacCode[:2] == "47" {
		return true
	}
	return retailMotorEACs[eacCode]
}
