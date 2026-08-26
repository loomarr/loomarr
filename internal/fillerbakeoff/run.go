package fillerbakeoff

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillereval"
)

const (
	maxPacketFacts   = 256
	maxPacketSignals = 64
	maxSignalText    = 1 << 20
	maxSignalBytes   = 12 << 20
	maxPacketBytes   = 64 << 20
	maxFrameWidth    = 1920
	maxFramePixels   = int64(maxFrameWidth * maxFrameWidth)
	maxVideoDuration = int64(60_000)
	maxRoutes        = 16
	maxFieldBytes    = 512
)

// Run executes one serial, label-blind cascade and returns captured predictions.
// It performs no scoring and grants no production admission authority.
func Run(ctx context.Context, config Config) ([]fillereval.Prediction, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	if config.Manifest.Kind != fillereval.CorpusCertification {
		return nil, fmt.Errorf("bakeoff requires a locked certification manifest")
	}
	if failures := fillereval.ValidateManifest(config.Manifest); len(failures) > 0 {
		return nil, fmt.Errorf("invalid bakeoff manifest: %s", strings.Join(failures, "; "))
	}
	selected := make(map[string]fillereval.Case)
	for _, c := range config.Manifest.Cases {
		if c.Split == config.Run.EvaluationSplit {
			selected[c.ID] = c
		}
	}
	if len(config.Packets) != len(selected) {
		return nil, fmt.Errorf("packet set has %d cases, selected split requires %d", len(config.Packets), len(selected))
	}
	for id, packet := range config.Packets {
		c, ok := selected[id]
		if !ok {
			return nil, fmt.Errorf("evidence packet %q has no case in selected split", id)
		}
		if err := validatePacket(c, packet, config.Run.EvidenceVersion, config.CorpusRoot); err != nil {
			return nil, err
		}
	}

	caseIDs := make([]string, 0, len(selected))
	for id := range selected {
		caseIDs = append(caseIDs, id)
	}
	slices.Sort(caseIDs)
	var predictions []fillereval.Prediction
	var spent int64
	var attempts int
	for _, id := range caseIDs {
		prediction, caseSpend, caseAttempts := runCase(ctx, config, config.Packets[id], spent, attempts)
		spent = saturatingAdd64(spent, caseSpend)
		attempts = saturatingAddInt(attempts, caseAttempts)
		predictions = append(predictions, prediction)
	}
	return predictions, nil
}

func runCase(ctx context.Context, config Config, packet Packet, spent int64, attempts int) (fillereval.Prediction, int64, int) {
	doc := filleradmission.Document{
		SchemaVersion: filleradmission.SchemaVersion, EvidenceVersion: packet.EvidenceVersion,
		PolicyVersion: config.Run.PolicyVersion, TaxonomyVersion: config.Run.TaxonomyVersion,
		ClipHash: packet.ContentSHA256, Evidence: slices.Clone(packet.Facts),
	}
	result := config.Policy.Evaluate(doc)
	prediction := fillereval.Prediction{CaseID: packet.CaseID}
	var steps []fillereval.InferenceStep
	caseSpend, caseAttempts := int64(0), 0
	for _, route := range config.Routes {
		if result.Hold != nil {
			prediction.OperationalFailure = string(result.Hold.Code) + ": " + result.Hold.Detail
			break
		}
		if result.Decision == nil || result.Decision.Verdict != filleradmission.VerdictReview || !matchesReason(route.EscalateOn, result.Decision.ReasonCodes) {
			if result.Decision != nil && result.Decision.Verdict != filleradmission.VerdictReview {
				break
			}
			continue
		}
		if exceedsIntCeiling(attempts, caseAttempts, route.MaxAttempts, config.Run.MaxRequests) {
			prediction.OperationalFailure = "budget_exhausted: request ceiling cannot reserve next rung"
			break
		}
		if exceedsInt64Ceiling(spent, caseSpend, route.MaxChargeNanoUSD, config.Run.MaxSpendNanoUSD) {
			prediction.OperationalFailure = "budget_exhausted: spend ceiling cannot reserve next rung"
			break
		}
		routedPacket, err := routePacket(packet, route)
		if err != nil {
			prediction.OperationalFailure = "route_unavailable: " + err.Error()
			break
		}
		signalData, err := loadSignalData(routedPacket, config.CorpusRoot)
		if err != nil {
			prediction.OperationalFailure = "extraction_failed: " + err.Error()
			break
		}
		extraction, err := config.Extractor.Extract(ctx, Request{Packet: routedPacket, Route: route, SignalData: signalData, Reasons: slices.Clone(result.Decision.ReasonCodes), Evidence: slices.Clone(doc.Evidence)})
		if err != nil {
			step := failedStep(route, extraction, err)
			steps = append(steps, step)
			caseSpend = saturatingAdd64(caseSpend, failedSpend(route, extraction))
			caseAttempts = saturatingAddInt(caseAttempts, step.Attempts)
			prediction.OperationalFailure = "extraction_failed: " + err.Error()
			break
		}
		step, err := capturedStep(route, extraction)
		if err != nil {
			failed := failedStep(route, extraction, err)
			steps = append(steps, failed)
			caseSpend = saturatingAdd64(caseSpend, failedSpend(route, extraction))
			caseAttempts = saturatingAddInt(caseAttempts, failed.Attempts)
			prediction.OperationalFailure = "evidence_invalid: " + err.Error()
			break
		}
		steps = append(steps, step)
		caseSpend = saturatingAdd64(caseSpend, step.ChargedNanoUSD)
		caseAttempts = saturatingAddInt(caseAttempts, step.Attempts)
		if step.ChargedNanoUSD > route.MaxChargeNanoUSD || step.Attempts > route.MaxAttempts {
			prediction.OperationalFailure = "budget_exhausted: provider exceeded reserved rung ceiling"
			break
		}
		doc.Evidence = append(doc.Evidence, extraction.Evidence...)
		doc.Attribution = append(doc.Attribution, extraction.Attribution)
		result = config.Policy.Evaluate(doc)
	}
	prediction.Steps = steps
	if prediction.OperationalFailure != "" {
		return prediction, caseSpend, caseAttempts
	}
	if result.Hold != nil {
		prediction.OperationalFailure = string(result.Hold.Code) + ": " + result.Hold.Detail
		return prediction, caseSpend, caseAttempts
	}
	if result.Decision == nil {
		prediction.OperationalFailure = "evidence_invalid: evaluator returned no terminal result"
		return prediction, caseSpend, caseAttempts
	}
	applyDecision(&prediction, *result.Decision, doc.Evidence)
	return prediction, caseSpend, caseAttempts
}

func exceedsInt64Ceiling(a, b, reservation, ceiling int64) bool {
	if a < 0 || b < 0 || reservation < 0 || a > ceiling || b > ceiling-a {
		return true
	}
	return reservation > ceiling-a-b
}

func exceedsIntCeiling(a, b, reservation, ceiling int) bool {
	if a < 0 || b < 0 || reservation < 0 || a > ceiling || b > ceiling-a {
		return true
	}
	return reservation > ceiling-a-b
}

func saturatingAdd64(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

func saturatingAddInt(a, b int) int {
	if b > 0 && a > math.MaxInt-b {
		return math.MaxInt
	}
	return a + b
}

func validateConfig(config Config) error {
	if config.Policy == nil || config.Extractor == nil {
		return fmt.Errorf("bakeoff requires an admission evaluator and extractor")
	}
	if config.Run.MaxRequests <= 0 || config.Run.MaxSpendNanoUSD <= 0 || config.Run.MaxConcurrency <= 0 {
		return fmt.Errorf("bakeoff requires positive request, spend, and concurrency ceilings")
	}
	if config.Run.EvaluationSplit != fillereval.SplitDevelopment && config.Run.EvaluationSplit != fillereval.SplitHoldout {
		return fmt.Errorf("bakeoff requires an explicit development or holdout split")
	}
	if config.Run.Profile == "" || config.Run.EvidenceVersion == "" || config.Run.PromptVersion == "" || config.Run.PolicyVersion == "" || config.Run.TaxonomyVersion == "" || config.Run.RolePolicyVersion == "" || config.Run.CapabilitySnapshot == "" || config.Run.PriceSnapshot == "" || config.Run.GeneratedAt.IsZero() {
		return fmt.Errorf("bakeoff requires complete version, snapshot, profile, and generation identity")
	}
	policyVersion, taxonomyVersion := config.Policy.PolicyIdentity()
	if policyVersion != config.Run.PolicyVersion || taxonomyVersion != config.Run.TaxonomyVersion {
		return fmt.Errorf("bakeoff run policy identity does not match the evaluator")
	}
	if len(config.Routes) == 0 || len(config.Routes) > maxRoutes {
		return fmt.Errorf("bakeoff requires between one and %d routes", maxRoutes)
	}
	previousRank := 0
	for i, route := range config.Routes {
		if !validRouteEnvelope(route) {
			return fmt.Errorf("route[%d] requires exact identity, modalities, positive reservations, and escalation reasons", i)
		}
		if strings.Contains(strings.ToLower(route.Model), "latest") || route.AllowFallbacks {
			return fmt.Errorf("route[%d] certification requires a concrete model with fallbacks disabled", i)
		}
		if route.Provider == "openrouter" && (route.UpstreamProviderSlug == "" || route.UpstreamProvider == "" || !route.StructuredOutput || !route.RequireZDR) {
			return fmt.Errorf("route[%d] OpenRouter certification requires provider selector and resolved identity pins, structured output, and ZDR", i)
		}
		if err := validateEscalation(route); err != nil {
			return fmt.Errorf("route[%d]: %w", i, err)
		}
		rank := routeRank(route.Class)
		if (i == 0 && route.Class != RouteText) || rank < previousRank || (previousRank == routeRank(RoutePremium) && rank == previousRank) {
			return fmt.Errorf("route[%d] violates text, frames, video, premium cascade order", i)
		}
		previousRank = rank
	}
	if slices.ContainsFunc(config.Routes, func(route Route) bool { return route.Provider == "openrouter" }) {
		if config.Snapshot == nil {
			return fmt.Errorf("OpenRouter bakeoff routes require a locked capability and price snapshot")
		}
		if err := ValidateOpenRouterRunSnapshot(config.Run, config.Routes, *config.Snapshot); err != nil {
			return err
		}
	}
	return nil
}

func validRouteEnvelope(route Route) bool {
	fields := []string{route.Role, route.Rung, route.Provider, route.Model}
	for _, field := range fields {
		if field == "" || len(field) > maxFieldBytes {
			return false
		}
	}
	return len(route.UpstreamProviderSlug) <= maxFieldBytes && len(route.UpstreamProvider) <= maxFieldBytes && len(route.MarginalValueEvidence) <= maxFieldBytes && len(route.MarginalValueSHA256) <= 64 &&
		len(route.Modalities) > 0 && len(route.Modalities) <= 8 &&
		route.MaxChargeNanoUSD > 0 && route.MaxAttempts == 1 &&
		len(route.EscalateOn) > 0 && len(route.EscalateOn) <= 32
}

func validatePacket(c fillereval.Case, packet Packet, evidenceVersion, corpusRoot string) error {
	if packet.SchemaVersion != PacketSchemaVersion || packet.CaseID != c.ID || packet.EvidenceVersion != evidenceVersion || packet.ContentSHA256 != c.ContentSHA256 {
		return fmt.Errorf("case %q evidence packet identity does not match run and manifest", c.ID)
	}
	if len(packet.Facts) > maxPacketFacts || len(packet.Signals) > maxPacketSignals {
		return fmt.Errorf("case %q evidence packet exceeds bounded item counts", c.ID)
	}
	for _, fact := range packet.Facts {
		validDeterministic := fact.Claim == filleradmission.ClaimMediaUsability && fact.Kind == filleradmission.KindDecoder ||
			fact.Claim == filleradmission.ClaimSourceLicense && fact.Kind == filleradmission.KindSourcePolicy
		if !validDeterministic || fact.EvaluationID != "" {
			return fmt.Errorf("case %q packet facts may contain only deterministic decoder and source-policy evidence", c.ID)
		}
	}
	var packetBytes int64
	frameCount, videoCount := 0, 0
	signalIDs := make(map[string]struct{}, len(packet.Signals))
	for _, signal := range packet.Signals {
		if signal.ID == "" || signal.Kind == "" || len(signal.ID) > maxFieldBytes || len(signal.Kind) > maxFieldBytes || len(signal.Path) > maxFieldBytes || len(signal.Text) > maxSignalText || len(signal.ContentTypes) > 8 || signal.Bytes < 0 || signal.DurationMS < 0 || signal.Width < 0 || signal.Height < 0 || signal.AtMS < 0 {
			return fmt.Errorf("case %q contains invalid bounded signal", c.ID)
		}
		if _, exists := signalIDs[signal.ID]; exists {
			return fmt.Errorf("case %q contains duplicate signal id %q", c.ID, signal.ID)
		}
		signalIDs[signal.ID] = struct{}{}
		for _, contentType := range signal.ContentTypes {
			if contentType == "" || len(contentType) > 128 {
				return fmt.Errorf("case %q contains invalid signal content type", c.ID)
			}
		}
		if !validSignalKind(signal.Kind) {
			return fmt.Errorf("case %q contains unsupported untrusted signal kind %q", c.ID, signal.Kind)
		}
		pathSignal := signal.Kind == string(filleradmission.KindFrame) || signal.Kind == string(filleradmission.KindAudio) || signal.Kind == string(filleradmission.KindVideo)
		if pathSignal && (signal.Path == "" || len(signal.SHA256) != 64 || signal.Bytes <= 0) {
			return fmt.Errorf("case %q external signal requires hash and positive byte bound", c.ID)
		}
		if signal.Bytes > maxSignalBytes || packetBytes > maxPacketBytes-signal.Bytes {
			return fmt.Errorf("case %q external signals exceed byte ceilings", c.ID)
		}
		packetBytes += signal.Bytes
		if !pathSignal && (signal.Text == "" || signal.Path != "") {
			return fmt.Errorf("case %q text signal requires inline text and no external path", c.ID)
		}
		if signal.Kind == string(filleradmission.KindFrame) {
			frameCount++
			if signal.Width <= 0 || signal.Height <= 0 || signal.Width > maxFrameWidth || int64(signal.Width)*int64(signal.Height) > maxFramePixels {
				return fmt.Errorf("case %q frame signal is outside pixel bounds", c.ID)
			}
		}
		if (signal.Kind == string(filleradmission.KindAudio) || signal.Kind == string(filleradmission.KindVideo)) && signal.DurationMS <= 0 {
			return fmt.Errorf("case %q temporal signal requires a positive duration bound", c.ID)
		}
		if signal.Kind == string(filleradmission.KindVideo) {
			videoCount++
			if signal.DurationMS > maxVideoDuration || signal.Width <= 0 || signal.Height <= 0 || signal.Width > 1280 || signal.Height > 720 {
				return fmt.Errorf("case %q video signal is outside duration or pixel bounds", c.ID)
			}
		}
		if pathSignal {
			if err := verifySignalFile(c.ID, signal, corpusRoot); err != nil {
				return err
			}
		}
	}
	if frameCount > 4 || videoCount > 1 {
		return fmt.Errorf("case %q exceeds frame or video derivative counts", c.ID)
	}
	if got := PacketSHA256(packet); got != c.EvidenceSHA256 {
		return fmt.Errorf("case %q evidence packet digest %s does not match manifest", c.ID, got)
	}
	return nil
}

func validSignalKind(kind string) bool {
	switch filleradmission.EvidenceKind(kind) {
	case filleradmission.KindRecordingSidecar, filleradmission.KindFilename,
		filleradmission.KindUploaderMetadata, filleradmission.KindTranscript,
		filleradmission.KindOCR, filleradmission.KindFrame,
		filleradmission.KindAudio, filleradmission.KindVideo:
		return true
	default:
		return false
	}
}

func verifySignalFile(caseID string, signal Signal, corpusRoot string) error {
	_, err := readSignalFile(caseID, signal, corpusRoot, false)
	return err
}

func loadSignalData(packet Packet, corpusRoot string) (map[string][]byte, error) {
	data := make(map[string][]byte)
	for _, signal := range packet.Signals {
		if signal.Path == "" {
			continue
		}
		value, err := readSignalFile(packet.CaseID, signal, corpusRoot, true)
		if err != nil {
			return nil, err
		}
		data[signal.ID] = value
	}
	return data, nil
}

func routePacket(packet Packet, route Route) (Packet, error) {
	routed := packet
	routed.Signals = make([]Signal, 0, len(packet.Signals))
	selectedModalities := make(map[string]int)
	for _, signal := range packet.Signals {
		modality := signalModality(signal.Kind)
		if slices.Contains(route.Modalities, modality) {
			routed.Signals = append(routed.Signals, signal)
			selectedModalities[modality]++
		}
	}
	for _, modality := range route.Modalities {
		if modality != "text" && modality != "image" && modality != "audio" && modality != "video" {
			return Packet{}, fmt.Errorf("route %q declares unsupported modality %q", route.Rung, modality)
		}
	}
	if route.Class == RouteFrames && selectedModalities["image"] == 0 {
		return Packet{}, fmt.Errorf("frame route %q has no verified image signal", route.Rung)
	}
	if route.Class == RouteVideo && selectedModalities["video"] == 0 {
		return Packet{}, fmt.Errorf("video route %q has no verified video signal", route.Rung)
	}
	return routed, nil
}

func signalModality(kind string) string {
	switch filleradmission.EvidenceKind(kind) {
	case filleradmission.KindFrame:
		return "image"
	case filleradmission.KindAudio:
		return "audio"
	case filleradmission.KindVideo:
		return "video"
	default:
		return "text"
	}
}

func readSignalFile(caseID string, signal Signal, corpusRoot string, retain bool) ([]byte, error) {
	root, err := filepath.Abs(corpusRoot)
	if err != nil || strings.TrimSpace(corpusRoot) == "" {
		return nil, fmt.Errorf("case %q external signals require a corpus root", caseID)
	}
	path := filepath.Join(root, filepath.Clean(signal.Path))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(signal.Path) {
		return nil, fmt.Errorf("case %q signal path escapes the corpus root", caseID)
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("case %q resolve corpus root: %w", caseID, err)
	}
	pathResolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("case %q resolve signal %q: %w", caseID, signal.ID, err)
	}
	resolvedRel, err := filepath.Rel(rootResolved, pathResolved)
	if err != nil || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("case %q signal symlink escapes the corpus root", caseID)
	}
	file, err := os.Open(pathResolved)
	if err != nil {
		return nil, fmt.Errorf("case %q open signal %q: %w", caseID, signal.ID, err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != signal.Bytes {
		return nil, fmt.Errorf("case %q signal %q size or file type does not match packet", caseID, signal.ID)
	}
	hash := sha256.New()
	var target io.Writer = hash
	var buffer bytes.Buffer
	if retain {
		buffer.Grow(int(signal.Bytes))
		target = io.MultiWriter(hash, &buffer)
	}
	read, err := io.Copy(target, io.LimitReader(file, signal.Bytes+1))
	if err != nil || read != signal.Bytes || hex.EncodeToString(hash.Sum(nil)) != signal.SHA256 {
		return nil, fmt.Errorf("case %q signal %q content does not match packet", caseID, signal.ID)
	}
	if !retain {
		return nil, nil
	}
	return buffer.Bytes(), nil
}

func validateEscalation(route Route) error {
	switch route.Class {
	case RouteText:
		if !slices.Equal(route.Modalities, []string{"text"}) {
			return fmt.Errorf("text route requires text-only input modalities")
		}
		if route.MarginalValueEvidence != "" || route.MarginalValueSHA256 != "" {
			return fmt.Errorf("text route cannot carry premium marginal-value evidence")
		}
	case RouteFrames:
		if route.Role != "filler_frames" || !slices.Contains(route.Modalities, "image") || slices.Contains(route.Modalities, "video") {
			return fmt.Errorf("frame route requires the filler_frames role and image input")
		}
		for _, reason := range route.EscalateOn {
			if !slices.Contains(frameReasons(), reason) {
				return fmt.Errorf("frames cannot escalate reason %q", reason)
			}
		}
		if route.MarginalValueEvidence != "" || route.MarginalValueSHA256 != "" {
			return fmt.Errorf("frame route cannot carry premium marginal-value evidence")
		}
	case RouteVideo:
		if route.Role != "filler_video" || !slices.Contains(route.Modalities, "video") {
			return fmt.Errorf("video route requires the filler_video role and video input")
		}
		if len(route.EscalateOn) != 1 || route.EscalateOn[0] != filleradmission.ReasonTemporalAmbiguity {
			return fmt.Errorf("video may run only for temporal ambiguity")
		}
		if route.MarginalValueEvidence != "" || route.MarginalValueSHA256 != "" {
			return fmt.Errorf("video route cannot carry premium marginal-value evidence")
		}
	case RoutePremium:
		if strings.TrimSpace(route.MarginalValueEvidence) == "" || !validSHA256(route.MarginalValueSHA256) {
			return fmt.Errorf("premium route requires a measured marginal-value artifact and SHA-256")
		}
	default:
		return fmt.Errorf("unknown route class %q", route.Class)
	}
	return nil
}

func routeRank(class RouteClass) int {
	switch class {
	case RouteText:
		return 1
	case RouteFrames:
		return 2
	case RouteVideo:
		return 3
	case RoutePremium:
		return 4
	default:
		return 0
	}
}

func validSHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func frameReasons() []filleradmission.ReasonCode {
	return []filleradmission.ReasonCode{
		filleradmission.ReasonMissingContentRole, filleradmission.ReasonInsufficientContentRole,
		filleradmission.ReasonMissingCommercialIdentity, filleradmission.ReasonInsufficientProductEvidence,
		filleradmission.ReasonInsufficientSensitiveEvidence, filleradmission.ReasonConflictBrand,
		filleradmission.ReasonConflictProduct, filleradmission.ReasonConflictContentRole,
	}
}

func PacketSHA256(packet Packet) string {
	data, err := json.Marshal(packet)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// DecodePacket rejects unknown or trailing fields so reviewed labels cannot be
// hidden in a provider input beside the closed raw-signal schema.
func DecodePacket(reader io.Reader) (Packet, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var packet Packet
	if err := decoder.Decode(&packet); err != nil {
		return Packet{}, fmt.Errorf("decode evidence packet: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Packet{}, fmt.Errorf("decode evidence packet: trailing JSON value")
		}
		return Packet{}, fmt.Errorf("decode evidence packet: %w", err)
	}
	return packet, nil
}

func matchesReason(allowed, actual []filleradmission.ReasonCode) bool {
	for _, reason := range actual {
		if slices.Contains(allowed, reason) {
			return true
		}
	}
	return false
}

func capturedStep(route Route, extraction Extraction) (fillereval.InferenceStep, error) {
	a := extraction.Attribution
	if a.EvaluationID == "" || a.Role != route.Role || a.Rung != route.Rung || a.RequestedProvider != route.Provider || a.RequestedModel != route.Model || a.ResolvedProvider == "" || a.ResolvedModel == "" || len(a.Modalities) == 0 || a.Attempts != 1 || a.UpstreamProvider != route.UpstreamProvider || !sameStrings(a.Modalities, route.Modalities) {
		return fillereval.InferenceStep{}, fmt.Errorf("provider attribution does not match reserved route")
	}
	nano := int64(0)
	if a.ChargedAmount != "" {
		if a.ChargedCurrency != "USD" {
			return fillereval.InferenceStep{}, fmt.Errorf("certification charge requires USD, got %q", a.ChargedCurrency)
		}
		var err error
		nano, err = fillereval.USDToNanoCeil(a.ChargedAmount)
		if err != nil {
			return fillereval.InferenceStep{}, err
		}
	} else if a.ChargedCurrency != "" {
		return fillereval.InferenceStep{}, fmt.Errorf("charged currency has no amount")
	}
	if a.Abstained != (strings.TrimSpace(a.AbstentionReason) != "") || (a.Abstained && len(extraction.Evidence) != 0) || (!a.Abstained && len(extraction.Evidence) == 0) {
		return fillereval.InferenceStep{}, fmt.Errorf("provider must return supported evidence or one explicit semantic abstention")
	}
	return fillereval.InferenceStep{
		EvaluationID: a.EvaluationID, Role: a.Role, Rung: a.Rung,
		RequestedProvider: a.RequestedProvider, RequestedModel: a.RequestedModel,
		ResolvedProvider: a.ResolvedProvider, ResolvedModel: a.ResolvedModel,
		UpstreamProvider: a.UpstreamProvider,
		Modalities:       slices.Clone(a.Modalities), Derivative: extraction.Derivative,
		Tokens:        tokenUsage(a),
		ChargedAmount: a.ChargedAmount, ChargedCurrency: a.ChargedCurrency, ChargedNanoUSD: nano,
		ReservedNanoUSD: route.MaxChargeNanoUSD, EstimatedNanoUSD: extraction.EstimatedNanoUSD, Attempts: a.Attempts,
		GenerationID: a.GenerationID, LatencyMS: a.LatencyMS,
		Abstained: a.Abstained, AbstentionReason: a.AbstentionReason,
	}, nil
}

func failedStep(route Route, extraction Extraction, extractionErr error) fillereval.InferenceStep {
	a := extraction.Attribution
	attempts := max(1, a.Attempts)
	failure := extractionErr.Error()
	if a.Attempts > 1 {
		failure += fmt.Sprintf("; adapter hid %d provider attempts in one invocation", a.Attempts)
	}
	evaluationID := a.EvaluationID
	if evaluationID == "" {
		evaluationID = "failed-" + route.Role + "-" + route.Rung
	}
	chargedNanoUSD := int64(0)
	if a.ChargedCurrency == "USD" {
		if exact, err := fillereval.USDToNanoCeil(a.ChargedAmount); err == nil {
			chargedNanoUSD = exact
		}
	}
	return fillereval.InferenceStep{
		EvaluationID: evaluationID, Role: route.Role, Rung: route.Rung,
		RequestedProvider: route.Provider, RequestedModel: route.Model,
		ResolvedProvider: a.ResolvedProvider, ResolvedModel: a.ResolvedModel,
		UpstreamProvider: route.UpstreamProvider, Modalities: slices.Clone(route.Modalities),
		Derivative: extraction.Derivative, Tokens: tokenUsage(a), ChargedAmount: a.ChargedAmount,
		ChargedCurrency: a.ChargedCurrency, ChargedNanoUSD: chargedNanoUSD,
		ReservedNanoUSD: route.MaxChargeNanoUSD, EstimatedNanoUSD: extraction.EstimatedNanoUSD, Attempts: attempts, GenerationID: a.GenerationID,
		LatencyMS: a.LatencyMS, OperationalFailure: failure,
	}
}

func failedSpend(route Route, extraction Extraction) int64 {
	a := extraction.Attribution
	if a.ChargedCurrency == "USD" {
		if exact, err := fillereval.USDToNanoCeil(a.ChargedAmount); err == nil {
			return exact
		}
	}
	return route.MaxChargeNanoUSD
}

func tokenUsage(a filleradmission.Attribution) fillereval.TokenUsage {
	return fillereval.TokenUsage{Prompt: int64(a.Tokens.Prompt), Completion: int64(a.Tokens.Completion), Reasoning: int64(a.Tokens.Reasoning), Cached: int64(a.Tokens.Cached), CacheWrite: int64(a.Tokens.CacheWrite), Image: int64(a.Tokens.Image), Audio: int64(a.Tokens.Audio), Video: int64(a.Tokens.Video)}
}

func sameStrings(actual, required []string) bool {
	return slices.Equal(uniqueSorted(actual), uniqueSorted(required))
}

func uniqueSorted(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}

func applyDecision(prediction *fillereval.Prediction, decision filleradmission.Decision, evidence []filleradmission.Evidence) {
	prediction.Verdict = fillereval.Verdict(decision.Verdict)
	prediction.ReasonCodes = make([]string, len(decision.ReasonCodes))
	for i, reason := range decision.ReasonCodes {
		prediction.ReasonCodes[i] = string(reason)
	}
	if decision.Verdict == filleradmission.VerdictReject {
		prediction.RejectClass = classifyReject(decision.ReasonCodes)
	}
	prediction.EvidenceRefs = slices.Clone(decision.EvidenceRefs)
	prediction.ReviewQuestion = decision.ReviewQuestion
	for _, conflict := range decision.Conflicts {
		prediction.Conflicts = append(prediction.Conflicts, fillereval.Conflict{Claim: string(conflict.Claim), Values: slices.Clone(conflict.Values), EvidenceRefs: slices.Clone(conflict.EvidenceRefs)})
	}
	for _, fact := range evidence {
		switch fact.Claim {
		case filleradmission.ClaimContentRole:
			if prediction.ContentRole == "" {
				prediction.ContentRole = fact.Value
			}
		case filleradmission.ClaimProduct:
			if prediction.Taxonomy == nil {
				prediction.Taxonomy = make(map[string][]string)
			}
			if !slices.Contains(prediction.Taxonomy["product"], fact.Value) {
				prediction.Taxonomy["product"] = append(prediction.Taxonomy["product"], fact.Value)
			}
		case filleradmission.ClaimSensitiveFlag:
			if !slices.Contains(prediction.PolicyFlags, fact.Value) {
				prediction.PolicyFlags = append(prediction.PolicyFlags, fact.Value)
			}
		}
	}
	for key := range prediction.Taxonomy {
		slices.Sort(prediction.Taxonomy[key])
	}
	slices.Sort(prediction.PolicyFlags)
}

func classifyReject(reasons []filleradmission.ReasonCode) fillereval.RejectClass {
	for _, reason := range reasons {
		switch reason {
		case filleradmission.ReasonMediaUnusable, filleradmission.ReasonSourceIneligible:
			continue
		default:
			return fillereval.RejectSemantic
		}
	}
	return fillereval.RejectDeterministic
}
