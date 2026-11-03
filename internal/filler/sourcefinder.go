package filler

import "errors"

var (
	ErrInvalidSourceReference = errors.New("invalid filler source reference")
	ErrSourceProvider         = errors.New("filler source provider unavailable")
)
