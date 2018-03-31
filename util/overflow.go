package util

// AddU32O adds 2 uint32s and checks if they overflowed
func AddU32O(a, b uint32) (uint32, bool) {
	return (a + b), !(a+b < a)
}

func SubU32O(a, b uint32) (uint32, bool) {
	return (a - b), !(a-b > a)
}
