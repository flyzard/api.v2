package domain

func (d *DraftFS) ValidateLimit() error {
	// Implement FS-specific validation logic here, such as checking document totals against FS limits based on the seller's EAC code.
	return nil
}

func IsRetailActivity(eacCode string) bool {
	if eacCode == "" || len(eacCode) != 5 {
		return false
	}

	if len(eacCode) >= 2 && eacCode[:2] == "47" {
		return true
	}

	return isMotorVehicleRetail(eacCode)
}

func isMotorVehicleRetail(eacCode string) bool {
	retailMotorCodes := map[string]bool{
		"45110": true,
		"45190": true,
		"45320": true,
		"45401": true,
		"45402": true,
	}
	return retailMotorCodes[eacCode]
}
