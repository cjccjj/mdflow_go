package commonmark

// RuneBoundarySplits returns deterministic boundaries after each UTF-8 rune.
// It is deliberately small and stable so diagnostics can distinguish parser
// differences from arbitrary byte chunking without trying every split plan.
func RuneBoundarySplits(input string) []int {
	splits := make([]int, 0, len(input))
	for i := range input {
		if i > 0 {
			splits = append(splits, i)
		}
	}
	return splits
}

// LineBoundarySplits returns deterministic boundaries immediately after each
// newline. It mirrors the normal production chunker behaviour.
func LineBoundarySplits(input string) []int {
	var splits []int
	for i := range input {
		if input[i] == '\n' && i+1 < len(input) {
			splits = append(splits, i+1)
		}
	}
	return splits
}
