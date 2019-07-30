package utils

// StrSlicesAreEqual returns a bool val if ss1 and ss2 contain the same elems
func StrSlicesAreEqual(ss1 []string, ss2 []string) bool {
	if len(ss1) != len(ss2) {
		return false
	}

	allVals := make(map[string]bool, 0)

	for _, val := range ss1 {
		allVals[val] = true
	}

	for _, val := range ss2 {
		allVals[val] = true
	}

	return len(allVals) == len(ss1)
}
