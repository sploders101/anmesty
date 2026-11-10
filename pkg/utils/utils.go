package utils

import "iter"

func LinearizeStringSet(set map[string]struct{}) []string {
	packages := make([]string, len(set))
	var i int
	for pkg := range set {
		packages[i] = pkg
		i++
	}
	return packages
}

func LinearizeIter[T any](set iter.Seq[T]) []T {
	var values []T
	for thing := range set {
		values = append(values, thing)
	}
	return values
}
