package vector

import "slices"

// Contains tells whether a slice `a` contains the element `x`.
func Contains[V comparable](a []V, x V) bool {
	return slices.Contains(a, x)
}
