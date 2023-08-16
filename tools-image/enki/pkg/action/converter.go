package action

// ConverterAction is the action that converts a non-kairos image to a Kairos one.
// The conversion happens in a best-effort manner. It's not guaranteed that
// any distribution will successfully be converted to a Kairos flavor. See
// the Kairos releases for known-to-work flavors.
type ConverterAction struct {
	rootFSPath string
}

func NewConverterAction() *ConverterAction {
	return &ConverterAction{}
}
