package dev

func contains(addr string, addrs []string) (found bool, index int) {
	for i, a := range addrs {
		if addr == a {
			return true, i
		}
	}
	return false, -1
}
