package playout

// mapSettings is a settings.Reader answering from a fixed map; a key it does not hold is
// unanswered (nil), so a handle reads that setting's declared default.
type mapSettings map[string]any

func (m mapSettings) Value(key string) any { return m[key] }
