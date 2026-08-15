package testkit

import (
	"context"

	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// ChannelCodecWrite records one revision-checked derived codec write.
type ChannelCodecWrite struct {
	ChannelID        string
	ExpectedRevision int64
	Codec            string
}

// ChannelCodecStore is the shared scripted double for codec measurement/write races.
// ReadRevisions and WriteErrors are consumed in order; the last read revision remains
// current once the script is exhausted.
type ChannelCodecStore struct {
	ReadRevisions []int64
	ReadCalls     int
	ReadError     error
	WriteErrors   []error
	Writes        []ChannelCodecWrite
}

func (s *ChannelCodecStore) GetChannel(_ context.Context, id string) (store.Channel, error) {
	if s.ReadError != nil {
		return store.Channel{}, s.ReadError
	}
	revision := int64(1)
	if len(s.ReadRevisions) > 0 {
		index := s.ReadCalls
		if index >= len(s.ReadRevisions) {
			index = len(s.ReadRevisions) - 1
		}
		revision = s.ReadRevisions[index]
	}
	s.ReadCalls++
	return store.Channel{Channel: schedule.Channel{ID: id}, Revision: revision}, nil
}

func (s *ChannelCodecStore) SetChannelBroadcastCodec(
	_ context.Context,
	id string,
	expectedRevision int64,
	codec string,
) (int64, error) {
	s.Writes = append(s.Writes, ChannelCodecWrite{
		ChannelID: id, ExpectedRevision: expectedRevision, Codec: codec,
	})
	if len(s.WriteErrors) > 0 {
		err := s.WriteErrors[0]
		s.WriteErrors = s.WriteErrors[1:]
		if err != nil {
			return 0, err
		}
	}
	return expectedRevision + 1, nil
}
