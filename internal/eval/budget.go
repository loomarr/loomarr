//go:build eval

package eval

import (
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
)

// CallBudget is the deterministic worst-case inference envelope reported before
// an evaluation constructs any external client.
type CallBudget struct {
	Cases                int                  `json:"cases"`
	Trials               int                  `json:"trials"`
	MaxGeneratorCalls    int                  `json:"maxGeneratorCalls"`
	MaxJudgeCalls        int                  `json:"maxJudgeCalls"`
	Total                int                  `json:"total"`
	Resource             ResourceBudget       `json:"resource"`
	GeneratorReservation InferenceReservation `json:"generatorReservation"`
	JudgeReservation     InferenceReservation `json:"judgeReservation"`
}

// ResourceBudget is the declared runtime inference ceiling. A run is one case
// trial; suite limits cover the complete Runner execution.
type ResourceBudget struct {
	MaxCallsPerRun    int    `json:"maxCallsPerRun"`
	MaxCallsPerSuite  int    `json:"maxCallsPerSuite"`
	MaxTokensPerRun   int    `json:"maxTokensPerRun"`
	MaxSpendPerRun    string `json:"maxSpendPerRun"`
	MaxTokensPerSuite int    `json:"maxTokensPerSuite"`
	MaxSpendPerSuite  string `json:"maxSpendPerSuite"`
}

// InferenceReservation is the conservative resource envelope that must fit
// before one hosted provider call may be dispatched.
type InferenceReservation struct {
	Tokens              int    `json:"tokens"`
	Spend               string `json:"spend"`
	MaxInputTokens      int    `json:"maxInputTokens,omitempty"`
	MaxCompletionTokens int    `json:"maxCompletionTokens,omitempty"`
}

// OpenRouterReservationConfig binds a resource reservation to one immutable
// capability snapshot and the exact request limits enforced by the caller.
type OpenRouterReservationConfig struct {
	Snapshot            fillerbakeoff.OpenRouterSnapshot
	SnapshotSHA256      string
	At                  time.Time
	Model               string
	UpstreamProvider    string
	MaxInputTokens      int
	MaxCompletionTokens int
	RequiredParameters  []string
}

// DeriveOpenRouterReservation prices one call against the most expensive
// active ZDR endpoint OpenRouter may choose inside the pinned provider family.
func DeriveOpenRouterReservation(config OpenRouterReservationConfig) (InferenceReservation, error) {
	if err := fillerbakeoff.ValidateOpenRouterSnapshot(config.Snapshot); err != nil {
		return InferenceReservation{}, err
	}
	if config.SnapshotSHA256 == "" || config.SnapshotSHA256 != fillerbakeoff.OpenRouterSnapshotSHA256(config.Snapshot) {
		return InferenceReservation{}, fmt.Errorf("OpenRouter snapshot digest does not match the pinned reservation authority")
	}
	if config.Snapshot.SourceBaseURL != OpenRouterCertificationBaseURL || config.At.IsZero() || config.At.Location() != time.UTC {
		return InferenceReservation{}, fmt.Errorf("OpenRouter reservation requires the canonical API snapshot and a UTC validation time")
	}
	age := config.At.Sub(config.Snapshot.RetrievedAt)
	if age < 0 || age > 24*time.Hour {
		return InferenceReservation{}, fmt.Errorf("OpenRouter reservation is outside the snapshot's 24-hour window")
	}
	if config.MaxInputTokens <= 0 || config.MaxCompletionTokens <= 0 {
		return InferenceReservation{}, fmt.Errorf("OpenRouter reservation requires positive input and completion limits")
	}
	tokens, ok := checkedAdd(config.MaxInputTokens, config.MaxCompletionTokens)
	if !ok {
		return InferenceReservation{}, fmt.Errorf("OpenRouter reservation token limit overflows")
	}
	var model *fillerbakeoff.OpenRouterModelSnapshot
	for index := range config.Snapshot.Models {
		if config.Snapshot.Models[index].ID == config.Model {
			model = &config.Snapshot.Models[index]
			break
		}
	}
	if model == nil {
		return InferenceReservation{}, fmt.Errorf("OpenRouter reservation model %q is absent from the snapshot", config.Model)
	}
	maximumCharge := int64(0)
	eligible := 0
	for _, endpoint := range model.Endpoints {
		if endpoint.ProviderName != config.UpstreamProvider || !endpoint.ZDR || endpoint.Status != 0 ||
			endpoint.MaxCompletionTokens < int64(config.MaxCompletionTokens) {
			continue
		}
		supported := true
		for _, parameter := range config.RequiredParameters {
			if !slices.Contains(endpoint.SupportedParameters, parameter) {
				supported = false
				break
			}
		}
		if !supported {
			continue
		}
		// The shared estimator prices every output token at completion price.
		// Fail closed if a route could charge reasoning tokens at a higher rate.
		if reasoning := endpoint.Pricing["internal_reasoning"]; reasoning != "" {
			reasoningPrice, reasoningOK := new(big.Rat).SetString(reasoning)
			completionPrice, completionOK := new(big.Rat).SetString(endpoint.Pricing["completion"])
			if !reasoningOK || !completionOK || reasoningPrice.Cmp(completionPrice) > 0 {
				return InferenceReservation{}, fmt.Errorf("OpenRouter route %q requires a stronger reasoning-token price bound", endpoint.ProviderSlug)
			}
		}
		charge, err := fillerbakeoff.EstimateOpenRouterTokenChargeNanoUSD(
			endpoint, int64(config.MaxInputTokens), int64(config.MaxCompletionTokens),
		)
		if err != nil {
			return InferenceReservation{}, fmt.Errorf("OpenRouter route %q reservation: %w", endpoint.ProviderSlug, err)
		}
		eligible++
		maximumCharge = max(maximumCharge, charge)
	}
	if eligible == 0 {
		return InferenceReservation{}, fmt.Errorf("OpenRouter reservation has no active ZDR endpoint for provider %q", config.UpstreamProvider)
	}
	return InferenceReservation{
		Tokens: tokens, Spend: nanoUSDDecimal(maximumCharge),
		MaxInputTokens: config.MaxInputTokens, MaxCompletionTokens: config.MaxCompletionTokens,
	}, nil
}

func nanoUSDDecimal(value int64) string {
	whole, fraction := value/1_000_000_000, value%1_000_000_000
	if fraction == 0 {
		return strconv.FormatInt(whole, 10)
	}
	return fmt.Sprintf("%d.%s", whole, strings.TrimRight(fmt.Sprintf("%09d", fraction), "0"))
}

type ResourceUsage struct {
	Calls  int    `json:"calls"`
	Tokens int    `json:"tokens"`
	Spend  string `json:"spend"`
}

// CertificationOptions are the resource decisions available before any Library,
// TMDB, generator, or judge client exists.
type CertificationOptions struct {
	Required                       bool
	LiveSchedule                   bool
	FrozenCatalog                  bool
	Trials                         int
	GeneratorProvider              string
	GeneratorBaseURL               string
	GeneratorModel                 string
	JudgeProvider                  string
	JudgeBaseURL                   string
	JudgeModel                     string
	GeneratorUpstream              string
	JudgeUpstream                  string
	AllowLocal                     bool
	MaxCallsPerRun                 string
	MaxCallsPerSuite               string
	MaxTokensPerRun                string
	MaxSpendPerRun                 string
	MaxTokensPerSuite              string
	MaxSpendPerSuite               string
	GeneratorTokensPerCall         string
	GeneratorSpendPerCall          string
	JudgeTokensPerCall             string
	JudgeSpendPerCall              string
	GeneratorOpenRouterReservation *OpenRouterReservationConfig
	JudgeOpenRouterReservation     *OpenRouterReservationConfig
}

// ParseEvaluationTrials resolves the run's trial count before any external
// client exists. Required certification rejects malformed explicit values;
// exploratory runs retain their forgiving one-trial default.
func ParseEvaluationTrials(required bool, raw string) (int, error) {
	defaultTrials := 1
	if required {
		defaultTrials = 3
	}
	if raw == "" {
		return defaultTrials, nil
	}
	trials, err := strconv.Atoi(raw)
	if err != nil || trials <= 0 {
		if required {
			return 0, fmt.Errorf("LOOMARR_EVAL_TRIALS must be a positive integer in certification mode")
		}
		return defaultTrials, nil
	}
	return trials, nil
}

// PrepareCertificationRun computes and validates the pre-provider call budget.
func PrepareCertificationRun(caseCount int, options CertificationOptions) (CallBudget, error) {
	trials := options.Trials
	if trials <= 0 {
		trials = 1
	}
	budget, budgetErr := computeCallBudget(caseCount, trials)
	if budgetErr != nil {
		return budget, budgetErr
	}
	if !options.Required {
		return budget, nil
	}
	resource, err := parseRequiredResourceBudget(options, budget)
	if err != nil {
		return budget, err
	}
	budget.Resource = resource
	if !options.LiveSchedule && !options.FrozenCatalog {
		return budget, fmt.Errorf("LOOMARR_EVAL_LIVE_SCHEDULE=1 is required in certification mode")
	}
	provider := strings.ToLower(strings.TrimSpace(options.GeneratorProvider))
	judgeProvider := strings.ToLower(strings.TrimSpace(options.JudgeProvider))
	if judgeProvider == "" {
		judgeProvider = provider
	}
	if (localInferenceProvider(provider) || localInferenceProvider(judgeProvider)) && !options.AllowLocal {
		return budget, fmt.Errorf("LOOMARR_EVAL_ALLOW_LOCAL=1 is required for local certification")
	}
	if provider == "openrouter" {
		if options.GeneratorBaseURL != OpenRouterCertificationBaseURL {
			return budget, fmt.Errorf("generator canonical OpenRouter API URL must be %s", OpenRouterCertificationBaseURL)
		}
		if err := llm.ValidateOpenRouterCertificationRoute(options.GeneratorModel, options.GeneratorUpstream); err != nil {
			return budget, fmt.Errorf("generator %w", err)
		}
	}
	if judgeProvider == "openrouter" {
		if options.JudgeBaseURL != OpenRouterCertificationBaseURL {
			return budget, fmt.Errorf("judge canonical OpenRouter API URL must be %s", OpenRouterCertificationBaseURL)
		}
		if err := llm.ValidateOpenRouterCertificationRoute(options.JudgeModel, options.JudgeUpstream); err != nil {
			return budget, fmt.Errorf("judge %w", err)
		}
	}
	if !localInferenceProvider(provider) {
		budget.GeneratorReservation, err = prepareRoleReservation(
			"generator", provider, options.GeneratorModel, options.GeneratorUpstream,
			options.GeneratorTokensPerCall, options.GeneratorSpendPerCall,
			options.GeneratorOpenRouterReservation, resource,
		)
		if err != nil {
			return budget, err
		}
	}
	if !localInferenceProvider(judgeProvider) {
		budget.JudgeReservation, err = prepareRoleReservation(
			"judge", judgeProvider, options.JudgeModel, options.JudgeUpstream,
			options.JudgeTokensPerCall, options.JudgeSpendPerCall,
			options.JudgeOpenRouterReservation, resource,
		)
		if err != nil {
			return budget, err
		}
	}
	return budget, nil
}

func prepareRoleReservation(role, provider, model, upstream, rawTokens, rawSpend string, openRouter *OpenRouterReservationConfig, limits ResourceBudget) (InferenceReservation, error) {
	if provider != "openrouter" {
		return parseRequiredInferenceReservation(role, rawTokens, rawSpend, limits)
	}
	if openRouter == nil {
		return InferenceReservation{}, fmt.Errorf("%s OpenRouter calls require a snapshot-derived reservation", role)
	}
	if openRouter.Model != model || openRouter.UpstreamProvider != upstream {
		return InferenceReservation{}, fmt.Errorf("%s OpenRouter reservation does not match the configured model and provider", role)
	}
	reservation, err := DeriveOpenRouterReservation(*openRouter)
	if err != nil {
		return InferenceReservation{}, fmt.Errorf("%s OpenRouter reservation: %w", role, err)
	}
	if reservation.Tokens > limits.MaxTokensPerRun || reservation.Tokens > limits.MaxTokensPerSuite {
		return InferenceReservation{}, fmt.Errorf("%s token reservation exceeds a declared ceiling", role)
	}
	spend, _ := parseExactDecimal(reservation.Spend)
	maxRunSpend, _ := parseExactDecimal(limits.MaxSpendPerRun)
	maxSuiteSpend, _ := parseExactDecimal(limits.MaxSpendPerSuite)
	if spend.cmp(maxRunSpend) > 0 || spend.cmp(maxSuiteSpend) > 0 {
		return InferenceReservation{}, fmt.Errorf("%s spend reservation exceeds a declared ceiling", role)
	}
	return reservation, nil
}

func parseRequiredInferenceReservation(role, rawTokens, rawSpend string, limits ResourceBudget) (InferenceReservation, error) {
	tokens, err := strconv.Atoi(rawTokens)
	if err != nil || tokens <= 0 {
		return InferenceReservation{}, fmt.Errorf("%s token reservation must be a positive integer in certification mode", role)
	}
	spend, valid := parseExactDecimal(rawSpend)
	if !valid || spend.sign() <= 0 {
		return InferenceReservation{}, fmt.Errorf("%s spend reservation must be a positive plain decimal in certification mode", role)
	}
	if tokens > limits.MaxTokensPerRun || tokens > limits.MaxTokensPerSuite {
		return InferenceReservation{}, fmt.Errorf("%s token reservation exceeds a declared ceiling", role)
	}
	maxRunSpend, _ := parseExactDecimal(limits.MaxSpendPerRun)
	maxSuiteSpend, _ := parseExactDecimal(limits.MaxSpendPerSuite)
	if spend.cmp(maxRunSpend) > 0 || spend.cmp(maxSuiteSpend) > 0 {
		return InferenceReservation{}, fmt.Errorf("%s spend reservation exceeds a declared ceiling", role)
	}
	return InferenceReservation{Tokens: tokens, Spend: rawSpend}, nil
}

func parseRequiredResourceBudget(options CertificationOptions, estimate CallBudget) (ResourceBudget, error) {
	parsePositiveInteger := func(name, raw string) (int, error) {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer in certification mode", name)
		}
		return value, nil
	}
	parseSpend := func(name, raw string) (string, error) {
		value, ok := parseExactDecimal(raw)
		if !ok || value.sign() <= 0 {
			return "", fmt.Errorf("%s must be a positive plain decimal in certification mode", name)
		}
		return raw, nil
	}
	var budget ResourceBudget
	var err error
	if budget.MaxCallsPerRun, err = parsePositiveInteger("LOOMARR_EVAL_MAX_CALLS_PER_RUN", options.MaxCallsPerRun); err != nil {
		return ResourceBudget{}, err
	}
	if budget.MaxCallsPerSuite, err = parsePositiveInteger("LOOMARR_EVAL_MAX_CALLS_PER_SUITE", options.MaxCallsPerSuite); err != nil {
		return ResourceBudget{}, err
	}
	perRunEstimate, ok := checkedAdd(suggest.ProductionBounds().MaxModelCalls, 1)
	if !ok {
		return ResourceBudget{}, fmt.Errorf("evaluation call budget overflow: generator bound plus judge call")
	}
	if budget.MaxCallsPerRun < perRunEstimate {
		return ResourceBudget{}, fmt.Errorf("LOOMARR_EVAL_MAX_CALLS_PER_RUN %d is below required worst-case run total %d", budget.MaxCallsPerRun, perRunEstimate)
	}
	if budget.MaxCallsPerSuite < estimate.Total {
		return ResourceBudget{}, fmt.Errorf("LOOMARR_EVAL_MAX_CALLS_PER_SUITE %d is below required worst-case suite total %d", budget.MaxCallsPerSuite, estimate.Total)
	}
	if budget.MaxCallsPerSuite < budget.MaxCallsPerRun {
		return ResourceBudget{}, fmt.Errorf("LOOMARR_EVAL_MAX_CALLS_PER_SUITE must be at least LOOMARR_EVAL_MAX_CALLS_PER_RUN")
	}
	if budget.MaxTokensPerRun, err = parsePositiveInteger("LOOMARR_EVAL_MAX_TOKENS_PER_RUN", options.MaxTokensPerRun); err != nil {
		return ResourceBudget{}, err
	}
	if budget.MaxSpendPerRun, err = parseSpend("LOOMARR_EVAL_MAX_SPEND_PER_RUN", options.MaxSpendPerRun); err != nil {
		return ResourceBudget{}, err
	}
	if budget.MaxTokensPerSuite, err = parsePositiveInteger("LOOMARR_EVAL_MAX_TOKENS", options.MaxTokensPerSuite); err != nil {
		return ResourceBudget{}, err
	}
	if budget.MaxSpendPerSuite, err = parseSpend("LOOMARR_EVAL_MAX_SPEND", options.MaxSpendPerSuite); err != nil {
		return ResourceBudget{}, err
	}
	if budget.MaxTokensPerSuite < budget.MaxTokensPerRun {
		return ResourceBudget{}, fmt.Errorf("LOOMARR_EVAL_MAX_TOKENS must be at least LOOMARR_EVAL_MAX_TOKENS_PER_RUN")
	}
	runSpend, _ := parseExactDecimal(budget.MaxSpendPerRun)
	suiteSpend, _ := parseExactDecimal(budget.MaxSpendPerSuite)
	if suiteSpend.cmp(runSpend) < 0 {
		return ResourceBudget{}, fmt.Errorf("LOOMARR_EVAL_MAX_SPEND must be at least LOOMARR_EVAL_MAX_SPEND_PER_RUN")
	}
	return budget, nil
}

type exactDecimal struct {
	coefficient *big.Int
	scale       int
}

func parseExactDecimal(raw string) (exactDecimal, bool) {
	if !validChargeAmount(raw) {
		return exactDecimal{}, false
	}
	parts := strings.SplitN(raw, ".", 2)
	digits := parts[0]
	scale := 0
	if len(parts) == 2 {
		digits += parts[1]
		scale = len(parts[1])
	}
	coefficient := new(big.Int)
	if _, ok := coefficient.SetString(digits, 10); !ok {
		return exactDecimal{}, false
	}
	return exactDecimal{coefficient: coefficient, scale: scale}, true
}

func zeroDecimal() exactDecimal { return exactDecimal{coefficient: new(big.Int)} }

func (d exactDecimal) sign() int { return d.coefficient.Sign() }

func (d exactDecimal) clone() exactDecimal {
	if d.coefficient == nil {
		return zeroDecimal()
	}
	return exactDecimal{coefficient: new(big.Int).Set(d.coefficient), scale: d.scale}
}

func (d exactDecimal) add(other exactDecimal) exactDecimal {
	scale := max(d.scale, other.scale)
	left := scaledDecimalCoefficient(d, scale)
	right := scaledDecimalCoefficient(other, scale)
	return exactDecimal{coefficient: new(big.Int).Add(left, right), scale: scale}
}

func (d exactDecimal) cmp(other exactDecimal) int {
	scale := max(d.scale, other.scale)
	return scaledDecimalCoefficient(d, scale).Cmp(scaledDecimalCoefficient(other, scale))
}

func (d exactDecimal) String() string {
	if d.coefficient == nil || d.coefficient.Sign() == 0 {
		return "0"
	}
	digits := d.coefficient.String()
	if d.scale == 0 {
		return digits
	}
	if len(digits) <= d.scale {
		digits = strings.Repeat("0", d.scale-len(digits)+1) + digits
	}
	cut := len(digits) - d.scale
	return digits[:cut] + "." + digits[cut:]
}

func scaledDecimalCoefficient(d exactDecimal, scale int) *big.Int {
	coefficient := new(big.Int).Set(d.coefficient)
	if scale > d.scale {
		coefficient.Mul(coefficient, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale-d.scale)), nil))
	}
	return coefficient
}

func localInferenceProvider(provider string) bool {
	return provider == "" || provider == "ollama"
}

func computeCallBudget(cases, trials int) (CallBudget, error) {
	bounds := suggest.ProductionBounds()
	budget := CallBudget{Cases: cases, Trials: trials}
	caseTrials, ok := checkedMultiply(cases, trials)
	if !ok {
		return budget, fmt.Errorf("evaluation call budget overflow: cases %d × trials %d", cases, trials)
	}
	budget.MaxJudgeCalls = caseTrials
	budget.MaxGeneratorCalls, ok = checkedMultiply(caseTrials, bounds.MaxModelCalls)
	if !ok {
		budget.MaxJudgeCalls = 0
		return budget, fmt.Errorf("evaluation call budget overflow: %d case-trials × %d generator calls", caseTrials, bounds.MaxModelCalls)
	}
	budget.Total, ok = checkedAdd(budget.MaxGeneratorCalls, budget.MaxJudgeCalls)
	if !ok {
		budget.MaxGeneratorCalls = 0
		budget.MaxJudgeCalls = 0
		return budget, fmt.Errorf("evaluation call budget overflow: generator and judge totals")
	}
	return budget, nil
}

func checkedMultiply(left, right int) (int, bool) {
	if left < 0 || right < 0 {
		return 0, false
	}
	maxInt := int(^uint(0) >> 1)
	if left != 0 && right > maxInt/left {
		return 0, false
	}
	return left * right, true
}

func checkedAdd(left, right int) (int, bool) {
	if left < 0 || right < 0 {
		return 0, false
	}
	maxInt := int(^uint(0) >> 1)
	if right > maxInt-left {
		return 0, false
	}
	return left + right, true
}
