package fillerreview

import (
	"strings"

	"github.com/loomarr/loomarr/internal/fillereval"
)

type temporalUnitWire struct {
	Kind              string   `json:"kind"`
	DecisiveSignalIDs []string `json:"decisiveSignalIds"`
	Reason            string   `json:"reason"`
}

type temporalRoleWire struct {
	Kind              string   `json:"kind"`
	DecisiveSignalIDs []string `json:"decisiveSignalIds"`
	Reason            string   `json:"reason"`
}

type temporalCallError struct {
	code      fillereval.TemporalFailureCode
	detail    string
	retryable bool
}

func (e *temporalCallError) Error() string { return e.detail }

func temporalUnitSchema(item TemporalReviewCase) map[string]any {
	return temporalClaimSchema(item, []string{"standalone", "compilation", "programme_excerpt", "unusable", "unclear"})
}

func temporalRoleSchema(item TemporalReviewCase) map[string]any {
	return temporalClaimSchema(item, []string{"commercial", "promo", "bumper", "psa", "station_id", "trailer", "interstitial", "unclear"})
}

func temporalHostedUnitSchema(item TemporalReviewCase) map[string]any {
	return temporalHostedClaimSchema(item, []string{"standalone", "compilation", "programme_excerpt", "unusable", "unclear"})
}

func temporalHostedRoleSchema(item TemporalReviewCase) map[string]any {
	return temporalHostedClaimSchema(item, []string{"commercial", "promo", "bumper", "psa", "station_id", "trailer", "interstitial", "unclear"})
}

func temporalClaimSchema(item TemporalReviewCase, kinds []string) map[string]any {
	properties := temporalClaimProperties(item, kinds)
	properties["reason"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 500}
	return map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"kind", "decisiveSignalIds", "reason"},
		"properties": properties,
	}
}

func temporalHostedClaimSchema(item TemporalReviewCase, kinds []string) map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"kind", "decisiveSignalIds"},
		"properties": temporalClaimProperties(item, kinds),
	}
}

func temporalClaimProperties(item TemporalReviewCase, kinds []string) map[string]any {
	signalIDs := make([]string, 0, len(item.Frames)*2+len(item.TranscriptSegments))
	for _, frame := range item.Frames {
		signalIDs = append(signalIDs, frame.ID)
		if frame.OCRSignalID != "" {
			signalIDs = append(signalIDs, frame.OCRSignalID)
		}
	}
	for _, segment := range item.TranscriptSegments {
		signalIDs = append(signalIDs, segment.ID)
	}
	// Keep the provider-facing grammar to the portable structured-output subset.
	// CoreWeave's Qwen grammar compiler advertises strict structured output but
	// rejects JSON Schema's uniqueItems keyword. DecisiveSignalIDs are set-like:
	// NormalizeTemporalAssessment canonicalizes duplicates before the same closed
	// evidence validator checks their count and membership.
	return map[string]any{
		"kind": map[string]any{"type": "string", "enum": kinds},
		"decisiveSignalIds": map[string]any{
			"type": "array", "minItems": 1, "maxItems": 4,
			"items": map[string]any{"type": "string", "enum": signalIDs},
		},
	}
}

func boundedTemporalDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) > 512 {
		return detail[:512]
	}
	if detail == "" {
		return "unspecified temporal assessment failure"
	}
	return detail
}

const temporalUnitTaskPrompt = `Assess the temporal structure of one identity-blind video span using only its ordered images, OCR, card hints, and transcript.

standalone means exactly one independently bounded, self-contained inserted item with one cohesive communicative structure. It may contain many shots, scenes, or excerpts when they belong to that one item. compilation means two or more separately bounded items or internal item boundaries; multiple shots, a title followed by scenes, or a montage inside one cohesive item is not enough. programme_excerpt means material that depends on a larger programme for its boundaries, including an ordinary scene, programme opening, performance recording, credit/title fragment, or other sustained programme material. An opening invitation or title card followed by sustained narrative material remains a programme opening unless the whole span has independent promotional framing. An abrupt ending during speech or action counts against a standalone boundary. unusable means corruption or degradation prevents reliable assessment of the span; a screen recapture, recording overlay, or poor image alone is not unusable while the temporal material remains assessable. unclear means the supplied evidence is insufficient to choose.

Continuity, a timestamp or PLAY overlay, a title/logo/copyright card, or the absence of cuts does not establish standalone framing. Conversely, scene changes do not establish a compilation. Compare the beginning, middle, and end for independent opening/closing structure. Do not classify semantic purpose and do not use a guessed purpose to force a standalone unit.

A frame id cites what is visibly present; its OCR signal id cites only machine-read text from that frame; a transcript id cites only that segment. Never infer source identity, prior labels, or facts absent from the package.`

const temporalRoleTaskPrompt = `Classify the semantic role of one video span already assessed as a standalone unit. Use only its ordered images, OCR, card hints, and transcript.

commercial means a product, brand, service, sponsorship, or paid proposition. A branded sponsorship message remains commercial even when it names the sponsored programme or channel. promo means an explicit preview, announcement, scheduling message, or invitation for a television programme, channel, episode, or broadcast without a product or sponsor proposition. bumper means a product-free break transition with transition framing; a timestamp, PLAY overlay, or absence of speech is not enough. psa means a public-interest appeal. Political, editorial, or propaganda material without a clear public-interest appeal is unclear, not commercial or PSA. station_id means station or network identification. trailer means an explicit preview or release proposition for a theatrical film. interstitial means other standalone connective filler only when no more specific role fits. unclear means the standalone role cannot be established.

A title, logo, named programme or film, copyright card, rating card, or narrative scene does not by itself establish commercial, promo, trailer, or bumper. Require evidence of the role's proposition or transition; choose unclear rather than inventing one.

A frame id cites what is visibly present; its OCR signal id cites only machine-read text from that frame; a transcript id cites only that segment. Never infer source identity, prior labels, or facts absent from the package.`

const temporalUnitSystemPrompt = temporalUnitTaskPrompt + `

Return exactly {"kind":"one closed value","decisiveSignalIds":["one to four supplied ids"],"reason":"one sentence"} with no other keys.

Cite one to four decisive supplied signal ids. A frame id cites what is visibly present; its OCR signal id cites only machine-read text from that frame; a transcript id cites only that segment. Keep the reason to one evidence-grounded sentence and do not use quotation marks inside it. Never infer source identity, prior labels, or facts absent from the package.`

const temporalRoleSystemPrompt = temporalRoleTaskPrompt + `

Return exactly {"kind":"one closed value","decisiveSignalIds":["one to four supplied ids"],"reason":"one sentence"} with no other keys.

Cite one to four decisive supplied signal ids. Keep the reason to one evidence-grounded sentence and do not use quotation marks inside it.`

const temporalHostedUnitSystemPrompt = temporalUnitTaskPrompt + `

Return exactly {"kind":"one closed value","decisiveSignalIds":["one to four supplied ids"]} with no other keys and no explanatory prose. Cite one to four decisive supplied signal ids.`

const temporalHostedRoleSystemPrompt = temporalRoleTaskPrompt + `

Return exactly {"kind":"one closed value","decisiveSignalIds":["one to four supplied ids"]} with no other keys and no explanatory prose. Cite one to four decisive supplied signal ids.`
