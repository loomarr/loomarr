package clipstore

// ClipReferenceColumns is clipReferences as "table.column", for the schema guard.
func ClipReferenceColumns() []string {
	out := make([]string, 0, len(clipReferences))
	for _, ref := range clipReferences {
		out = append(out, ref.table+"."+ref.column)
	}
	return out
}
