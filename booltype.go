package cloudwatcher

type Bool bool

func (bit *Bool) UnmarshalJSON(b []byte) error {
	txt := string(b)
	*bit = Bool(txt == "1" || txt == "true")
	return nil
}

func (bit *Bool) MarshalJSON() ([]byte, error) {
	if *bit {
		return []byte("true"), nil
	}
	return []byte("false"), nil
}
