package httpcore

// ChannelIDInput is the path parameter every /v1/channels/{id}/… operation takes.
type ChannelIDInput struct {
	ID string `path:"id" example:"ch_abc123"`
}
