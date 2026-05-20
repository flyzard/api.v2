package domain

type DocumentType string

const (
	// Sales
	FT DocumentType = "FT"
	FS DocumentType = "FS"
	FR DocumentType = "FR"
	NC DocumentType = "NC"
	ND DocumentType = "ND"

	//   Transport
	GT DocumentType = "GT"
	GR DocumentType = "GR"
	GA DocumentType = "GA"
	GC DocumentType = "GC"
	GD DocumentType = "GD"

	//   Working
	OR DocumentType = "OR"
	PF DocumentType = "PF"
	NE DocumentType = "NE"
	CM DocumentType = "CM"
	FC DocumentType = "FC"
	DC DocumentType = "DC"
	FO DocumentType = "FO"

	//   Receipts
	RC DocumentType = "RC"
	RG DocumentType = "RG"
)

func (dt DocumentType) IsValid() bool {
	switch dt {
	case FT, FS, FR, NC, ND, GT, GR, GA, GC, GD, OR, PF, NE, CM, FC, DC, FO, RC, RG:
		return true
	default:
		return false
	}
}

func (dt DocumentType) IsSales() bool {
	switch dt {
	case FT, FS, FR, NC, ND:
		return true
	default:
		return false
	}
}

func (dt DocumentType) IsTransport() bool {
	switch dt {
	case GT, GR, GA, GC, GD:
		return true
	default:
		return false
	}
}

func (dt DocumentType) IsWorking() bool {
	switch dt {
	case OR, PF, NE, CM, FC, DC, FO:
		return true
	default:
		return false
	}
}

func (dt DocumentType) IsReceipt() bool {
	switch dt {
	case RC, RG:
		return true
	default:
		return false
	}
}

func (dt DocumentType) RequiresRef() bool {
	switch dt {
	case NC, ND:
		return true
	default:
		return false
	}
}

func (dt DocumentType) SupportsSelfBilling() bool {
	switch dt {
	case FT, FS, FR:
		return true
	default:
		return false
	}
}
