package orders

// ProcessOrder has early exits, short-circuit guards, loop transfers and switch
// alternatives without requiring any dependencies or executing target code.
func ProcessOrder(items []int, approved bool) int {
	if !approved || len(items) == 0 {
		return 0
	}
	total := 0
items:
	for i := range items {
		if items[i] < 0 {
			continue items
		}
		switch items[i] {
		case 0:
			continue
		case 99:
			break items
		default:
			total += items[i]
		}
		if total > 1000 {
			return total
		}
	}
	return total
}
