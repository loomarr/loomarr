package pod

// Year is a single-year era, the shape most pod tests set.
func Year(y int) EraRange { return EraRange{From: y, To: y} }
