// Package diagnostics records bounded, redacted technical evidence for Loomarr's operator and
// support surfaces (§17).
//
// Activity remains the curated product-history feed. Diagnostics instead owns structured server
// and client observations plus the lifecycle metadata for external media processes. Recording is
// always best-effort: saturation or persistence failure may lose evidence, but can never fail the
// request, Job, reconcile, or Playout operation being observed.
package diagnostics
