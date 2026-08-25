package filleradmission

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxEvidenceFacts     = 256
	maxAttributionItems  = 16
	maxOperationalIssues = 16
	maxIDBytes           = 128
	maxValueBytes        = 512
	maxSourceBytes       = 256
	maxLocationBytes     = 512
	maxDetailBytes       = 512
)

// Evaluator is the policy-owning admission seam. It is deliberately pure: all
// extraction, inference, persistence, retries, and clocks stay outside it.
type Evaluator struct {
	policy Policy
	sets   policySets
}

type policySets struct {
	products, roles, sensitive, prohibited map[string]struct{}
}

func New(policy Policy) (*Evaluator, error) {
	if strings.TrimSpace(policy.Version) == "" || strings.TrimSpace(policy.TaxonomyVersion) == "" ||
		!bounded(policy.Version, maxIDBytes) || !bounded(policy.TaxonomyVersion, maxIDBytes) {
		return nil, fmt.Errorf("filler admission policy requires policy and taxonomy versions")
	}
	if len(policy.AllowedProducts) > 4096 || len(policy.AllowedContentRoles) > 16 ||
		len(policy.KnownSensitiveFlags) > 256 || len(policy.ProhibitedFlags) > 256 {
		return nil, fmt.Errorf("filler admission policy exceeds bounded taxonomy counts")
	}
	for _, value := range append(append([]string(nil), policy.AllowedProducts...), policy.KnownSensitiveFlags...) {
		if !canonicalSlug(value) {
			return nil, fmt.Errorf("filler admission taxonomy value %q is not canonical", value)
		}
	}
	for _, value := range policy.ProhibitedFlags {
		if !canonicalSlug(value) {
			return nil, fmt.Errorf("filler admission prohibited flag %q is not canonical", value)
		}
	}
	sets := policySets{
		products:   stringSet(policy.AllowedProducts),
		roles:      stringSet(policy.AllowedContentRoles),
		sensitive:  stringSet(policy.KnownSensitiveFlags),
		prohibited: stringSet(policy.ProhibitedFlags),
	}
	if len(sets.roles) == 0 {
		return nil, fmt.Errorf("filler admission policy requires at least one allowed content role")
	}
	for _, role := range sortedKeys(sets.roles) {
		if !admissibleRole(role) {
			return nil, fmt.Errorf("content role %q cannot be declared eligible filler", role)
		}
	}
	for _, flag := range sortedKeys(sets.prohibited) {
		if _, ok := sets.sensitive[flag]; !ok {
			return nil, fmt.Errorf("prohibited sensitive flag %q is not in the known taxonomy", flag)
		}
	}
	return &Evaluator{policy: policy, sets: sets}, nil
}

// Evaluate validates one complete evidence document and returns either a
// semantic decision or an operational hold. It never returns both.
func (e *Evaluator) Evaluate(doc Document) Result {
	attribution := stableAttribution(doc.Attribution)
	if code, detail := e.validateEnvelope(doc); code != "" {
		return operational(code, detail, false, attribution)
	}
	if len(doc.Operational) > 0 {
		issues := append([]OperationalIssue(nil), doc.Operational...)
		slices.SortFunc(issues, func(a, b OperationalIssue) int {
			if n := strings.Compare(string(a.Code), string(b.Code)); n != 0 {
				return n
			}
			return strings.Compare(a.Detail, b.Detail)
		})
		issue := issues[0]
		return operational(issue.Code, issue.Detail, issue.Retryable, attribution)
	}
	if code, detail := e.validateEvidence(doc.Evidence); code != "" {
		return operational(code, detail, false, attribution)
	}

	facts := append([]Evidence(nil), doc.Evidence...)
	slices.SortFunc(facts, func(a, b Evidence) int { return strings.Compare(a.ID, b.ID) })
	conflicts, unresolved := findConflicts(facts)
	unresolvedClaims := make(map[Claim]struct{}, len(unresolved))
	for _, conflict := range unresolved {
		unresolvedClaims[conflict.Claim] = struct{}{}
	}

	roleFacts := factsForClaim(facts, ClaimContentRole)
	role := ""
	var roleMatches []Evidence
	if len(roleFacts) > 0 {
		role = roleFacts[0].Value
		roleMatches = matchingFacts(roleFacts, ClaimContentRole, role)
	}

	var rejectReasons []ReasonCode
	var rejectRefs []string
	if _, conflicted := unresolvedClaims[ClaimMediaUsability]; !conflicted {
		if refs := matchingRefs(facts, ClaimMediaUsability, UsabilityUnusable); len(refs) > 0 {
			rejectReasons = append(rejectReasons, ReasonMediaUnusable)
			rejectRefs = append(rejectRefs, refs...)
		}
	}
	if _, conflicted := unresolvedClaims[ClaimSourceLicense]; !conflicted {
		if refs := matchingRefs(facts, ClaimSourceLicense, EligibilityIneligible); len(refs) > 0 {
			rejectReasons = append(rejectReasons, ReasonSourceIneligible)
			rejectRefs = append(rejectRefs, refs...)
		}
	}
	prohibited, uncertainProhibited := e.prohibitedRefs(facts)
	if len(prohibited) > 0 {
		rejectReasons = append(rejectReasons, ReasonSensitivePolicyProhibited)
		rejectRefs = append(rejectRefs, prohibited...)
	}
	_, roleConflicted := unresolvedClaims[ClaimContentRole]
	if !roleConflicted && corroboratedSemantic(roleMatches) {
		if _, allowed := e.sets.roles[role]; !allowed {
			rejectReasons = append(rejectReasons, ReasonContentRoleNotFiller)
			rejectRefs = append(rejectRefs, ids(roleMatches)...)
		}
	}
	if len(rejectReasons) > 0 {
		return semantic(VerdictReject, rejectReasons, rejectRefs, conflicts, "", attribution)
	}
	if len(unresolved) > 0 {
		reasons := make([]ReasonCode, 0, len(unresolved))
		refs := make([]string, 0)
		for _, conflict := range unresolved {
			reasons = append(reasons, conflictReason(conflict.Claim))
			refs = append(refs, conflict.EvidenceRefs...)
		}
		return semantic(VerdictReview, reasons, refs, conflicts, reviewQuestion(unresolved[0].Claim), attribution)
	}
	if len(uncertainProhibited) > 0 {
		return semantic(VerdictReview, []ReasonCode{ReasonInsufficientSensitiveEvidence}, uncertainProhibited, conflicts,
			"Does this clip contain the flagged sensitive material?", attribution)
	}

	usable := matchingFacts(facts, ClaimMediaUsability, UsabilityUsable)
	if len(usable) == 0 {
		return semantic(VerdictReview, []ReasonCode{ReasonMissingMediaUsability}, nil, conflicts,
			"Can this file be decoded and played correctly?", attribution)
	}
	eligible := matchingFacts(facts, ClaimSourceLicense, EligibilityEligible)
	if len(eligible) == 0 {
		return semantic(VerdictReview, []ReasonCode{ReasonMissingSourceLicense}, nil, conflicts,
			"Is this source and item licensed for use as filler?", attribution)
	}

	if len(roleFacts) == 0 {
		return semantic(VerdictReview, []ReasonCode{ReasonMissingContentRole}, nil, conflicts,
			"What kind of filler is this clip?", attribution)
	}
	if !corroboratedSemantic(roleMatches) {
		return semantic(VerdictReview, []ReasonCode{ReasonInsufficientContentRole}, ids(roleMatches), conflicts,
			"Is this clip a commercial, bumper, PSA, or station ID?", attribution)
	}
	refs := append(ids(usable), ids(eligible)...)
	refs = append(refs, ids(roleMatches)...)
	if role == RoleCommercial {
		productFacts := factsForClaim(facts, ClaimProduct)
		productOK := corroboratedSemantic(productFacts)
		switch {
		case productOK:
			refs = append(refs, ids(productFacts)...)
		case len(productFacts) > 0:
			return semantic(VerdictReview, []ReasonCode{ReasonInsufficientProductEvidence}, ids(productFacts), conflicts,
				"What product is this commercial advertising?", attribution)
		default:
			return semantic(VerdictReview, []ReasonCode{ReasonMissingCommercialIdentity}, nil, conflicts,
				"What product is this commercial advertising?", attribution)
		}
	}
	return semantic(VerdictAdmit, []ReasonCode{ReasonEvidenceSatisfied}, refs, conflicts, "", attribution)
}

func (e *Evaluator) validateEnvelope(doc Document) (OperationalCode, string) {
	if doc.SchemaVersion != SchemaVersion {
		return HoldSchemaInvalid, fmt.Sprintf("evidence schema %d does not match %d", doc.SchemaVersion, SchemaVersion)
	}
	if doc.ClipHash == "" || doc.EvidenceVersion == "" ||
		!bounded(doc.ClipHash, maxIDBytes) || !bounded(doc.EvidenceVersion, maxIDBytes) {
		return HoldEvidenceInvalid, "clip hash and evidence version are required"
	}
	if len(doc.Evidence) > maxEvidenceFacts || len(doc.Attribution) > maxAttributionItems || len(doc.Operational) > maxOperationalIssues {
		return HoldEvidenceInvalid, "evidence document exceeds bounded item counts"
	}
	if doc.PolicyVersion != e.policy.Version {
		return HoldSchemaInvalid, "admission policy version does not match evaluator"
	}
	if doc.TaxonomyVersion != e.policy.TaxonomyVersion {
		return HoldTaxonomyInvalid, "taxonomy version does not match evaluator"
	}
	seenAttribution := make(map[string]struct{}, len(doc.Attribution))
	for _, item := range doc.Attribution {
		if item.EvaluationID == "" || item.Role == "" || item.RequestedProvider == "" || item.RequestedModel == "" ||
			!bounded(item.EvaluationID, maxIDBytes) ||
			!bounded(item.Role, maxIDBytes) || !bounded(item.Rung, maxIDBytes) ||
			!bounded(item.RequestedProvider, maxIDBytes) || !bounded(item.RequestedModel, maxSourceBytes) ||
			!bounded(item.ResolvedProvider, maxIDBytes) || !bounded(item.ResolvedModel, maxSourceBytes) ||
			!bounded(item.UpstreamProvider, maxIDBytes) || !bounded(item.GenerationID, maxIDBytes) ||
			!bounded(item.ChargedAmount, maxIDBytes) || !bounded(item.ChargedCurrency, 16) ||
			len(item.Modalities) > 8 || item.LatencyMS < 0 || item.Attempts < 0 || negativeTokens(item.Tokens) ||
			((item.ChargedAmount == "") != (item.ChargedCurrency == "")) {
			return HoldEvidenceInvalid, "attribution is incomplete or outside its bounds"
		}
		for _, modality := range item.Modalities {
			if modality == "" || !bounded(modality, maxIDBytes) {
				return HoldEvidenceInvalid, "attribution contains an invalid modality"
			}
		}
		if _, exists := seenAttribution[item.EvaluationID]; exists {
			return HoldEvidenceInvalid, "attribution evaluation ids must be unique"
		}
		seenAttribution[item.EvaluationID] = struct{}{}
	}
	usedAttribution := make(map[string]struct{}, len(seenAttribution))
	for _, fact := range doc.Evidence {
		if fact.EvaluationID == "" {
			continue
		}
		if _, exists := seenAttribution[fact.EvaluationID]; !exists {
			return HoldEvidenceInvalid, fmt.Sprintf("evidence %q references unknown inference evaluation", fact.ID)
		}
		usedAttribution[fact.EvaluationID] = struct{}{}
	}
	if len(doc.Operational) == 0 {
		for id := range seenAttribution {
			if _, used := usedAttribution[id]; !used {
				return HoldEvidenceInvalid, fmt.Sprintf("inference evaluation %q is not referenced by evidence", id)
			}
		}
	}
	for _, issue := range doc.Operational {
		if !validOperationalCode(issue.Code) {
			return HoldSchemaInvalid, fmt.Sprintf("unknown operational state %q", issue.Code)
		}
		if !bounded(issue.Detail, maxDetailBytes) {
			return HoldEvidenceInvalid, "operational detail exceeds its bound"
		}
	}
	return "", ""
}

func (e *Evaluator) validateEvidence(evidence []Evidence) (OperationalCode, string) {
	seen := make(map[string]struct{}, len(evidence))
	for _, fact := range evidence {
		if fact.ID == "" || fact.Source == "" || fact.Value == "" ||
			!bounded(fact.ID, maxIDBytes) || !bounded(fact.Value, maxValueBytes) ||
			!bounded(fact.Source, maxSourceBytes) || !bounded(fact.Derivative, maxIDBytes) ||
			!bounded(fact.Location, maxLocationBytes) || !bounded(fact.EvaluationID, maxIDBytes) || fact.AtMS < 0 {
			return HoldEvidenceInvalid, "every evidence fact requires id, source, and value"
		}
		if _, exists := seen[fact.ID]; exists {
			return HoldEvidenceInvalid, "evidence ids must be unique"
		}
		seen[fact.ID] = struct{}{}
		if !validClaim(fact.Claim) || !validKind(fact.Kind) || !kindCanSupport(fact.Claim, fact.Kind) {
			return HoldEvidenceInvalid, fmt.Sprintf("unsupported evidence contract for %q", fact.ID)
		}
		if code := e.validateValue(fact); code != "" {
			return code, fmt.Sprintf("invalid %s value on evidence %q", fact.Claim, fact.ID)
		}
	}
	return "", ""
}

func (e *Evaluator) validateValue(fact Evidence) OperationalCode {
	switch fact.Claim {
	case ClaimMediaUsability:
		if fact.Value != UsabilityUsable && fact.Value != UsabilityUnusable {
			return HoldEvidenceInvalid
		}
	case ClaimSourceLicense:
		if fact.Value != EligibilityEligible && fact.Value != EligibilityIneligible {
			return HoldEvidenceInvalid
		}
	case ClaimRecordingDate:
		year, err := strconv.Atoi(fact.Value)
		if err != nil || year < 1880 || year > 2100 {
			return HoldEvidenceInvalid
		}
	case ClaimContentRole:
		if !knownRole(fact.Value) {
			return HoldEvidenceInvalid
		}
	case ClaimProduct:
		if _, ok := e.sets.products[fact.Value]; !ok {
			return HoldTaxonomyInvalid
		}
	case ClaimSensitiveFlag:
		if _, ok := e.sets.sensitive[fact.Value]; !ok {
			return HoldTaxonomyInvalid
		}
	case ClaimBrand:
		if len(fact.Value) > 200 {
			return HoldEvidenceInvalid
		}
	}
	return ""
}

func findConflicts(facts []Evidence) ([]Conflict, []Conflict) {
	var conflicts, unresolved []Conflict
	for _, claim := range conflictClaims() {
		claimFacts := factsForClaim(facts, claim)
		values := uniqueValues(claimFacts)
		if len(values) < 2 {
			continue
		}
		maxAuthority := 0
		for _, fact := range claimFacts {
			maxAuthority = max(maxAuthority, authority(fact))
		}
		var strongest []Evidence
		for _, fact := range claimFacts {
			if authority(fact) == maxAuthority {
				strongest = append(strongest, fact)
			}
		}
		resolved := len(uniqueValues(strongest)) == 1 && maxAuthority > minimumAuthority(claimFacts)
		conflict := Conflict{Claim: claim, Values: values, EvidenceRefs: ids(claimFacts), Resolved: resolved}
		if resolved {
			conflict.ResolvedBy = ids(strongest)
		} else {
			unresolved = append(unresolved, conflict)
		}
		conflicts = append(conflicts, conflict)
	}
	return conflicts, unresolved
}

func authority(fact Evidence) int {
	switch fact.Claim {
	case ClaimMediaUsability:
		if fact.Kind == KindDecoder {
			return 100
		}
	case ClaimSourceLicense:
		if fact.Kind == KindSourcePolicy {
			return 100
		}
	case ClaimRecordingDate:
		if fact.Kind == KindRecordingSidecar || fact.Kind == KindSourcePolicy {
			return 100
		}
	}
	return 10
}

func minimumAuthority(facts []Evidence) int {
	if len(facts) == 0 {
		return 0
	}
	low := authority(facts[0])
	for _, fact := range facts[1:] {
		low = min(low, authority(fact))
	}
	return low
}

func kindCanSupport(claim Claim, kind EvidenceKind) bool {
	switch claim {
	case ClaimMediaUsability:
		return kind == KindDecoder
	case ClaimSourceLicense:
		return kind == KindSourcePolicy
	case ClaimRecordingDate:
		return kind == KindSourcePolicy || kind == KindRecordingSidecar || kind == KindFilename ||
			kind == KindUploaderMetadata || kind == KindTranscript || kind == KindOCR || kind == KindFrame || kind == KindAudio || kind == KindVideo
	case ClaimBrand, ClaimProduct, ClaimContentRole:
		return kind == KindRecordingSidecar || kind == KindFilename || kind == KindUploaderMetadata ||
			kind == KindTranscript || kind == KindOCR || kind == KindFrame || kind == KindAudio || kind == KindVideo
	case ClaimSensitiveFlag:
		return kind == KindSourcePolicy || kind == KindTranscript || kind == KindOCR || kind == KindFrame || kind == KindAudio || kind == KindVideo
	default:
		return false
	}
}

func (e *Evaluator) prohibitedRefs(facts []Evidence) (accepted, uncertain []string) {
	byValue := make(map[string][]Evidence)
	for _, fact := range factsForClaim(facts, ClaimSensitiveFlag) {
		if _, prohibited := e.sets.prohibited[fact.Value]; prohibited {
			byValue[fact.Value] = append(byValue[fact.Value], fact)
		}
	}
	for _, value := range sortedKeys(byValue) {
		valueFacts := byValue[value]
		authoritative := false
		for _, fact := range valueFacts {
			authoritative = authoritative || fact.Kind == KindSourcePolicy
		}
		if authoritative || corroborated(valueFacts) {
			accepted = append(accepted, ids(valueFacts)...)
		} else {
			uncertain = append(uncertain, ids(valueFacts)...)
		}
	}
	return uniqueStrings(accepted), uniqueStrings(uncertain)
}

func semantic(verdict Verdict, reasons []ReasonCode, refs []string, conflicts []Conflict, question string, attribution []Attribution) Result {
	stableReasons(reasons)
	refs = uniqueStrings(refs)
	return Result{Decision: &Decision{
		Verdict: verdict, ReasonCodes: reasons, EvidenceRefs: refs, Conflicts: conflicts,
		ReviewQuestion: question, Attribution: attribution,
	}}
}

func operational(code OperationalCode, detail string, retryable bool, attribution []Attribution) Result {
	return Result{Hold: &Hold{Code: code, Detail: detail, Retryable: retryable, Attribution: attribution}}
}

func corroborated(facts []Evidence) bool {
	if len(uniqueValues(facts)) != 1 {
		return false
	}
	return independentKinds(facts) >= 2
}

func corroboratedSemantic(facts []Evidence) bool {
	if !corroborated(facts) {
		return false
	}
	for _, fact := range facts {
		switch fact.Kind {
		case KindTranscript, KindOCR, KindFrame, KindAudio, KindVideo:
			return true
		}
	}
	return false
}

func independentKinds(facts []Evidence) int {
	kinds := make(map[EvidenceKind]struct{}, len(facts))
	origins := make(map[string]struct{}, len(facts))
	generations := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		kinds[fact.Kind] = struct{}{}
		origin := fact.Source + "\x00" + fact.Derivative
		origins[origin] = struct{}{}
		generation := "deterministic\x00" + string(fact.Kind) + "\x00" + origin
		if fact.EvaluationID != "" {
			generation = "inference\x00" + fact.EvaluationID
		}
		generations[generation] = struct{}{}
	}
	return min(len(kinds), len(origins), len(generations))
}

func factsForClaim(facts []Evidence, claim Claim) []Evidence {
	var out []Evidence
	for _, fact := range facts {
		if fact.Claim == claim {
			out = append(out, fact)
		}
	}
	return out
}

func matchingFacts(facts []Evidence, claim Claim, value string) []Evidence {
	var out []Evidence
	for _, fact := range facts {
		if fact.Claim == claim && fact.Value == value {
			out = append(out, fact)
		}
	}
	return out
}

func matchingRefs(facts []Evidence, claim Claim, value string) []string {
	return ids(matchingFacts(facts, claim, value))
}

func ids(facts []Evidence) []string {
	refs := make([]string, 0, len(facts))
	for _, fact := range facts {
		refs = append(refs, fact.ID)
	}
	return uniqueStrings(refs)
}

func uniqueValues(facts []Evidence) []string {
	values := make([]string, 0, len(facts))
	for _, fact := range facts {
		values = append(values, fact.Value)
	}
	return uniqueStrings(values)
}

func uniqueStrings(values []string) []string {
	slices.Sort(values)
	return slices.Compact(values)
}

func stableReasons(reasons []ReasonCode) {
	slices.Sort(reasons)
}

func stableAttribution(in []Attribution) []Attribution {
	out := append([]Attribution(nil), in...)
	for i := range out {
		out[i].Modalities = uniqueStrings(append([]string(nil), out[i].Modalities...))
	}
	slices.SortFunc(out, func(a, b Attribution) int { return strings.Compare(a.EvaluationID, b.EvaluationID) })
	return out
}

func conflictReason(claim Claim) ReasonCode {
	switch claim {
	case ClaimMediaUsability:
		return ReasonConflictMediaUsability
	case ClaimRecordingDate:
		return ReasonConflictRecordingDate
	case ClaimBrand:
		return ReasonConflictBrand
	case ClaimProduct:
		return ReasonConflictProduct
	case ClaimContentRole:
		return ReasonConflictContentRole
	case ClaimSourceLicense:
		return ReasonConflictSourceLicense
	default:
		panic("unreachable admission claim")
	}
}

func reviewQuestion(claim Claim) string {
	switch claim {
	case ClaimMediaUsability:
		return "Can this file be decoded and played correctly?"
	case ClaimRecordingDate:
		return "Which date describes when this clip was recorded?"
	case ClaimBrand:
		return "Which brand is this clip advertising?"
	case ClaimProduct:
		return "Which product is this clip advertising?"
	case ClaimContentRole:
		return "Is this clip a commercial, bumper, PSA, station ID, trailer, interstitial, programme excerpt, or compilation?"
	case ClaimSourceLicense:
		return "Is this source and item licensed for use as filler?"
	default:
		return "What fact is missing or contradictory?"
	}
}

func allClaims() []Claim {
	return []Claim{ClaimMediaUsability, ClaimRecordingDate, ClaimBrand, ClaimProduct, ClaimContentRole, ClaimSourceLicense, ClaimSensitiveFlag}
}

// Sensitive flags form a set: adult and violence may both be present. They are
// therefore not competing values of one scalar claim. A future negative
// assertion needs its own subject/value schema before it can conflict safely.
func conflictClaims() []Claim {
	return []Claim{ClaimMediaUsability, ClaimSourceLicense, ClaimContentRole, ClaimProduct, ClaimBrand, ClaimRecordingDate}
}

func validClaim(claim Claim) bool { return slices.Contains(allClaims(), claim) }

func validKind(kind EvidenceKind) bool {
	return slices.Contains([]EvidenceKind{
		KindDecoder, KindSourcePolicy, KindRecordingSidecar, KindFilename, KindUploaderMetadata,
		KindTranscript, KindOCR, KindFrame, KindAudio, KindVideo,
	}, kind)
}

func validOperationalCode(code OperationalCode) bool {
	return slices.Contains([]OperationalCode{
		HoldProviderUnavailable, HoldBudgetExhausted, HoldExtractionFailed, HoldRouteUnavailable,
		HoldSchemaInvalid, HoldTaxonomyInvalid, HoldEvidenceInvalid,
	}, code)
}

func knownRole(role string) bool {
	return slices.Contains([]string{
		RoleCommercial, RoleBumper, RolePSA, RoleStationID, RoleTrailer, RoleInterstitial,
		RoleProgrammeExcerpt, RoleCompilation,
	}, role)
}

func admissibleRole(role string) bool {
	return slices.Contains([]string{RoleCommercial, RoleBumper, RolePSA, RoleStationID, RoleTrailer, RoleInterstitial}, role)
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func bounded(value string, maxBytes int) bool {
	return len(value) <= maxBytes && utf8.ValidString(value)
}

func canonicalSlug(value string) bool {
	if value == "" || !bounded(value, maxIDBytes) {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func negativeTokens(tokens TokenUsage) bool {
	return tokens.Prompt < 0 || tokens.Completion < 0 || tokens.Reasoning < 0 || tokens.Cached < 0 ||
		tokens.CacheWrite < 0 || tokens.Image < 0 || tokens.Audio < 0 || tokens.Video < 0
}
