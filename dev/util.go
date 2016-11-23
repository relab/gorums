package dev

func contains(addr string, addrs []string) (found bool, index int) {
	for i, a := range addrs {
		if addr == a {
			return true, i
		}
	}
	return false, -1
}

func addIfNotPresent(x uint32, set []uint32) {
	for _, y := range set {
		if y == x {
			return
		}
	}
	set = append(set, x)
}
