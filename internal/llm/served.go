package llm

import "context"

// ServedModels lists the model ids and aliases an OpenAI-compatible endpoint answers to, from
// GET {baseURL}/models. An error means the endpoint could not say (no such route, auth, down) —
// not that it serves nothing — so a caller must treat it as "unknown", never as "absent".
func ServedModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	models, err := fetchModels(ctx, baseURL, apiKey, false)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, m := range models {
		names = append(names, m.ID)
		names = append(names, m.Aliases...)
	}
	return names, nil
}
