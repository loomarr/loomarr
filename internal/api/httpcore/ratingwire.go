package httpcore

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/platform/rating"
)

// RatingWire is a content rating a client may send or be offered (#3012): one named component
// listing rating.Ladder, so the generated client carries the ratings as an enum instead of each
// screen keeping its own list. It is NOT aliased onto rating.Rating globally: a channel policy's
// ceiling also travels as "" (no limit), which an enum would refuse.
type RatingWire string

const ratingSchema = "Rating"

// Schema declares the shared Rating component and returns a reference to it.
func (RatingWire) Schema(r huma.Registry) *huma.Schema {
	if _, ok := r.Map()[ratingSchema]; !ok {
		values := make([]any, 0, len(rating.Ladder()))
		for _, value := range rating.Ladder() {
			values = append(values, string(value))
		}
		r.Map()[ratingSchema] = &huma.Schema{
			Type:        huma.TypeString,
			Enum:        values,
			Description: "A content rating on Loomarr's audience ladder, TV and film systems side by side, lowest first.",
		}
	}
	return &huma.Schema{Ref: "#/components/schemas/" + ratingSchema}
}
