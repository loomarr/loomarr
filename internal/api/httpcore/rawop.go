package httpcore

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// Raw-byte operations (§7.1) — routes whose handler owns the ResponseWriter.
//
// Some responses are not typed JSON: a database snapshot, an image, a video answering Range
// requests, a 302 carrying Set-Cookie. That is a fact about the RESPONSE SHAPE, and it is not a
// reason to leave the route off the API. Registering these here rather than on the bare mux is
// what puts them behind the single authorization middleware, the CSRF check, and the request-id
// error stamping that every other operation already gets.
//
// ⚠ **Plain-mux handlers duplicate auth per handler and sit outside the SPA's origin**, and two
// authorization paths is exactly the shape that once let two hand-written guards disagree about
// whether a nil authorizer denies or allows. Hence one path for every route.
//
// Handler bodies keep their `(w, r)` signature; only the mounting differs from a typed operation.

// RawOp registers one raw-byte operation: method + path + required role, an optional middleware
// chain, and the stdlib handler that writes the response itself.
//
// ⚠ **The type parameter is not decoration.** Huma builds the spec's `parameters` list from the
// input struct and does NOT cross-check it against the `{placeholders}` in Path. An operation
// registered with `struct{}` on a path like `/v1/filler/thumb/{hash}` therefore emits a path retired-ok
// template whose parameter is declared nowhere — invalid OpenAPI 3.1, and a generated client
// that cannot fill in the URL. A Hidden route can hide the omission, but only until it is
// unhidden. Declare the params; the handler may still read them via r.PathValue.
func RawOp[I any](
	api huma.API,
	op huma.Operation,
	role Role,
	body func(http.ResponseWriter, *http.Request),
	mw ...func(huma.Context, func(huma.Context)),
) {
	RawInputOp(api, op, role, func(w http.ResponseWriter, r *http.Request, _ *I) {
		body(w, r)
	}, mw...)
}

// RawInputOp is RawOp's typed-input form. Huma still owns parameter parsing and validation, while
// the response body retains direct access to ResponseWriter for negotiated or streamed formats.
func RawInputOp[I any](
	api huma.API,
	op huma.Operation,
	role Role,
	body func(http.ResponseWriter, *http.Request, *I),
	mw ...func(huma.Context, func(huma.Context)),
) {
	// ⚠ Every raw op declares its role out loud, same as every typed op. roleForOperation
	// defaults to ADMIN for an unmarked operation, so a route that must stay reachable without
	// a session (the channel icon Tunarr fetches machine-to-machine) has to say RolePublic —
	// forgetting fails closed, which surfaces as a 403 in development rather than an exposure.
	op = WithRole(op, role)
	op.Middlewares = append(op.Middlewares, mw...)
	huma.Register(api, op, func(_ context.Context, input *I) (*huma.StreamResponse, error) {
		return &huma.StreamResponse{Body: func(hctx huma.Context) {
			r, w := humago.Unwrap(hctx)
			body(w, r, input)
		}}, nil
	})
}

// BytesResponse declares the 200 for an opaque byte body so the spec says `image/jpeg` rather
// than implying JSON.
//
// Huma fills in a response it finds empty but never overwrites content already set here — it
// guards on `len(op.Responses[status].Content) == 0` — so this wins.
//
// Several content types are allowed because one route can legitimately serve more than one: the
// HLS asset route returns a playlist or a segment depending on which file was asked for, and a
// spec claiming only one of those would be a documented lie.
func BytesResponse(op huma.Operation, description string, contentTypes ...string) huma.Operation {
	if op.Responses == nil {
		op.Responses = map[string]*huma.Response{}
	}
	content := make(map[string]*huma.MediaType, len(contentTypes))
	for _, ct := range contentTypes {
		content[ct] = &huma.MediaType{Schema: &huma.Schema{Type: huma.TypeString, Format: "binary"}}
	}
	op.Responses["200"] = &huma.Response{Description: description, Content: content}
	return op
}

// HashInput addresses a clip by its CONTENT HASH (V45a) — hex, no slashes, so a plain `{hash}`
// segment needs no wildcard and no encoding. Shared by the thumbnail, media and hover routes,
// which are the same lookup three times over.
type HashInput struct {
	Hash string `path:"hash" doc:"The clip's content hash (from ClipDTO)" example:"9f2b1c4e8a7d6503"`
}
