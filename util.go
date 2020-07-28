package gorums

func appendIfNotPresent(set []uint32, x uint32) []uint32 {
	for _, y := range set {
		if y == x {
			return set
		}
	}
	return append(set, x)
}
