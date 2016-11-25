package dev

func contains(addr string, addrs []string) (found bool, index int) {
	for i, a := range addrs {
		if addr == a {
			return true, i
		}
	}
	return false, -1
}

func appendIfNotPresent(set []uint32, x uint32) []uint32 {
	for _, y := range set {
		if y == x {
			return set
		}
	}
	return append(set, x)
}
