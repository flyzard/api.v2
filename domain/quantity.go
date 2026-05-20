package domain

type Quantity int64

func ParseQuantity(s string) (Quantity, error) {
	v, err := parseFixed(s, 5)
	if err != nil {
		return 0, err
	}
	return Quantity(v), nil
}

func (q Quantity) String() string { return formatFixed5(int64(q)) }

func (q Quantity) MarshalJSON() ([]byte, error) {
	return []byte(`"` + q.String() + `"`), nil
}

func (q *Quantity) UnmarshalJSON(data []byte) error {
	v, err := ParseQuantity(unquoteJSONNumber(data))
	if err != nil {
		return err
	}
	*q = v
	return nil
}
