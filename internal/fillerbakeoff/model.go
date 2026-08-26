// Package fillerbakeoff runs bounded, inference-spending filler admission
// comparisons. It never owns admission policy or certification scoring.
package fillerbakeoff

import (
	"context"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillereval"
)

const PacketSchemaVersion = 1

// Packet is the label-blind, content-addressed input to provider extraction.
// Media remains external and is referenced only by bounded, hashed derivatives.
type Packet struct {
	SchemaVersion   int                        `json:"schemaVersion"`
	CaseID          string                     `json:"caseId"`
	EvidenceVersion string                     `json:"evidenceVersion"`
	ContentSHA256   string                     `json:"contentSha256"`
	Facts           []filleradmission.Evidence `json:"facts"`
	Signals         []Signal                   `json:"signals,omitempty"`
}

type Signal struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Text         string   `json:"text,omitempty"`
	Path         string   `json:"path,omitempty"`
	SHA256       string   `json:"sha256,omitempty"`
	Bytes        int64    `json:"bytes,omitempty"`
	DurationMS   int64    `json:"durationMs,omitempty"`
	Width        int      `json:"width,omitempty"`
	Height       int      `json:"height,omitempty"`
	AtMS         int64    `json:"atMs,omitempty"`
	ContentTypes []string `json:"contentTypes,omitempty"`
}

type RouteClass string

const (
	RouteText    RouteClass = "text"
	RouteFrames  RouteClass = "frames"
	RouteVideo   RouteClass = "video"
	RoutePremium RouteClass = "premium"
)

type Route struct {
	Class                RouteClass `json:"class"`
	Role                 string     `json:"role"`
	Rung                 string     `json:"rung"`
	Provider             string     `json:"provider"`
	Model                string     `json:"model"`
	UpstreamProviderSlug string     `json:"upstreamProviderSlug,omitempty"`
	UpstreamProvider     string     `json:"upstreamProvider,omitempty"`
	Modalities           []string   `json:"modalities"`
	StructuredOutput     bool       `json:"structuredOutput"`
	RequireZDR           bool       `json:"requireZdr"`
	AllowFallbacks       bool       `json:"allowFallbacks"`
	// MarginalValueEvidence names the measured artifact that justified a
	// premium route's incremental ceiling. It is forbidden on cheaper rungs.
	MarginalValueEvidence string                       `json:"marginalValueEvidence,omitempty"`
	MarginalValueSHA256   string                       `json:"marginalValueSha256,omitempty"`
	MaxChargeNanoUSD      int64                        `json:"maxChargeNanoUsd"`
	MaxAttempts           int                          `json:"maxAttempts"`
	EscalateOn            []filleradmission.ReasonCode `json:"escalateOn"`
}

type Request struct {
	Packet Packet
	Route  Route
	// SignalData contains the verified bytes for every external signal. Provider
	// adapters must use these bytes and never reopen Packet.Signal paths.
	SignalData map[string][]byte
	Reasons    []filleradmission.ReasonCode
	Evidence   []filleradmission.Evidence
}

type Extraction struct {
	Evidence         []filleradmission.Evidence
	Attribution      filleradmission.Attribution
	Derivative       fillereval.Derivative
	EstimatedNanoUSD int64
}

type Extractor interface {
	// Extract performs exactly one provider attempt. Adapters must not retry
	// internally: each retry needs its own runner reservation and ledger step.
	Extract(context.Context, Request) (Extraction, error)
}

type AdmissionEvaluator interface {
	Evaluate(filleradmission.Document) filleradmission.Result
	PolicyIdentity() (policyVersion, taxonomyVersion string)
}

type Config struct {
	Run        fillereval.RunIdentity
	Manifest   fillereval.Manifest
	Packets    map[string]Packet
	CorpusRoot string
	Policy     AdmissionEvaluator
	Routes     []Route
	Extractor  Extractor
}
