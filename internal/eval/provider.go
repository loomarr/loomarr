//go:build eval

package eval

import (
	"fmt"
	"strings"

	"github.com/loomarr/loomarr/internal/llm"
)

const OpenRouterCertificationBaseURL = "https://openrouter.ai/api/v1"

// CertificationProviderConfig is the complete prevalidated transport input for
// one generator or judge role. UpstreamProvider is meaningful only for OpenRouter.
type CertificationProviderConfig struct {
	Provider         string
	BaseURL          string
	Model            string
	APIKey           string
	UpstreamProvider string
}

// NewCertificationProvider constructs one inference role without falling back
// from a strict OpenRouter route to generic OpenAI-compatible behavior.
func NewCertificationProvider(cfg CertificationProviderConfig) (llm.Provider, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("certification provider requires a model")
	}
	if provider == "" || provider == "ollama" {
		return llm.NewOllama(cfg.BaseURL, cfg.Model), nil
	}
	if provider == "openrouter" {
		return llm.NewOpenRouterChat(llm.OpenRouterChatConfig{
			BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey, UpstreamProvider: cfg.UpstreamProvider,
		})
	}
	return llm.NewOpenAIForProvider(provider, cfg.BaseURL, cfg.Model, cfg.APIKey), nil
}
