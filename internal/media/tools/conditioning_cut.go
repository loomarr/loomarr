package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const conditioningEdgeWindowMs int64 = 3_000

type conditioningPacketProbe struct {
	Streams []conditioningProbeStreamJSON `json:"streams"`
	Packets []struct {
		StreamIndex *int   `json:"stream_index"`
		PTS         string `json:"pts_time"`
		Duration    string `json:"duration_time"`
		Flags       string `json:"flags"`
		DataHash    string `json:"data_hash"`
	} `json:"packets"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

type conditioningPacket struct {
	kind     StreamKind
	index    int
	pts      OptionalMilliseconds
	duration OptionalMilliseconds
	hash     string
}

func (t *FFmpegTools) measureConditioningCut(ctx context.Context, childPath, parentPath string, childContainerStartMs, childDurationMs int64, childStreams []ConditioningStream, intended Interval) (ConditioningCutMeasurement, error) {
	measured := ConditioningCutMeasurement{}
	// The child's edge windows follow its container timeline, which starts at format.start_time
	// (never read before zero, as before #1540) and ends format.duration later.
	childTimelineStart := max(int64(0), childContainerStartMs)
	childTimelineEnd, err := checkedConditioningAdd(childContainerStartMs, childDurationMs)
	if err != nil {
		return measured, err
	}
	if childTimelineEnd <= childTimelineStart {
		return measured, fmt.Errorf("%w: child container timeline is empty", ErrConditioningOutput)
	}
	childStart, err := t.conditioningPackets(ctx, childPath, childTimelineStart, min(childTimelineStart+conditioningEdgeWindowMs, childTimelineEnd))
	if err != nil {
		return measured, fmt.Errorf("measure child start edge: %w", err)
	}
	childEndStart := max(childTimelineStart, childTimelineEnd-conditioningEdgeWindowMs)
	childEnd, err := t.conditioningPackets(ctx, childPath, childEndStart, childTimelineEnd)
	if err != nil {
		return measured, fmt.Errorf("measure child end edge: %w", err)
	}
	startUpper, err := checkedConditioningAdd(intended.StartMs, conditioningEdgeWindowMs)
	if err != nil {
		return measured, err
	}
	endUpper, err := checkedConditioningAdd(intended.EndMs, conditioningEdgeWindowMs)
	if err != nil {
		return measured, err
	}
	parentStart, err := t.conditioningPackets(ctx, parentPath, max(int64(0), intended.StartMs-conditioningEdgeWindowMs), startUpper)
	if err != nil {
		return measured, fmt.Errorf("measure parent start edge: %w", err)
	}
	parentEnd, err := t.conditioningPackets(ctx, parentPath, max(int64(0), intended.EndMs-conditioningEdgeWindowMs), endUpper)
	if err != nil {
		return measured, fmt.Errorf("measure parent end edge: %w", err)
	}

	for _, stream := range childStreams {
		out := ConditioningCutStream{Kind: stream.Kind, Index: stream.Index}
		if child := earliestPresentedPacket(childStart, stream.Kind, stream.Index); child != nil {
			if parent := uniquePacketWithHash(parentStart, stream.Kind, stream.Index, child.hash); parent != nil {
				value, subErr := checkedConditioningSub(parent.pts.Milliseconds, intended.StartMs)
				if subErr != nil {
					return measured, subErr
				}
				out.StartError = OptionalMilliseconds{Milliseconds: value, Available: true}
			}
		}
		child, latestErr := latestPresentedPacket(childEnd, stream.Kind, stream.Index)
		if latestErr != nil {
			return measured, latestErr
		}
		if child != nil {
			if parent := uniquePacketWithHash(parentEnd, stream.Kind, stream.Index, child.hash); parent != nil && parent.duration.Available {
				parentEndMs, addErr := checkedConditioningAdd(parent.pts.Milliseconds, parent.duration.Milliseconds)
				if addErr != nil {
					return measured, addErr
				}
				value, subErr := checkedConditioningSub(parentEndMs, intended.EndMs)
				if subErr != nil {
					return measured, subErr
				}
				out.EndError = OptionalMilliseconds{Milliseconds: value, Available: true}
			}
		}
		measured.Streams = append(measured.Streams, out)
	}
	return measured, nil
}

func (t *FFmpegTools) conditioningPackets(ctx context.Context, path string, startMs, endMs int64) ([]conditioningPacket, error) {
	if endMs <= startMs {
		return nil, fmt.Errorf("%w: packet interval must be positive", ErrConditioningOutput)
	}
	interval := msToSeconds(startMs) + "%" + msToSeconds(endMs)
	raw, err := runConditioningCommand(ctx, t.FFprobePath, conditioningProbeOutputLimit, false,
		"-v", "error", "-read_intervals", interval, "-show_packets", "-show_streams", "-show_format", "-show_data_hash", "sha256",
		"-show_entries", "stream=index,codec_type:packet=stream_index,pts_time,duration_time,flags,data_hash:format=duration", "-of", "json", path)
	if err != nil {
		return nil, err
	}
	var probed conditioningPacketProbe
	if err := json.Unmarshal(raw, &probed); err != nil {
		return nil, fmt.Errorf("%w: parse packet JSON: %v", ErrConditioningOutput, err)
	}
	streams, err := validateConditioningStreamProjection(probed.Streams)
	if err != nil {
		return nil, err
	}
	_, err = parseConditioningContainerDuration(probed.Format.Duration)
	if err != nil {
		return nil, fmt.Errorf("parent/child packet probe: %w", err)
	}
	kinds := make(map[int]StreamKind, len(streams))
	for _, stream := range streams {
		kinds[stream.index] = stream.kind
	}
	packets := make([]conditioningPacket, 0, len(probed.Packets))
	for _, packet := range probed.Packets {
		if packet.StreamIndex == nil || *packet.StreamIndex < 0 {
			return nil, fmt.Errorf("%w: packet has missing or negative stream index", ErrConditioningOutput)
		}
		index := *packet.StreamIndex
		kind, known := kinds[index]
		if !known || strings.Contains(packet.Flags, "D") {
			continue
		}
		pts, err := parseConditioningMilliseconds(packet.PTS)
		if err != nil {
			return nil, fmt.Errorf("%w: packet stream %d PTS", err, index)
		}
		packetDuration, err := parseConditioningDuration(packet.Duration)
		if err != nil {
			return nil, fmt.Errorf("%w: packet stream %d duration", err, index)
		}
		if packetDuration.Available && packetDuration.Milliseconds == 0 {
			packetDuration = OptionalMilliseconds{}
		}
		if !pts.Available || packet.DataHash == "" {
			continue
		}
		packets = append(packets, conditioningPacket{kind: kind, index: index, pts: pts, duration: packetDuration, hash: packet.DataHash})
	}
	return packets, nil
}

func earliestPresentedPacket(packets []conditioningPacket, kind StreamKind, index int) *conditioningPacket {
	var earliest *conditioningPacket
	for i := range packets {
		if packets[i].kind == kind && packets[i].index == index && (earliest == nil || packets[i].pts.Milliseconds < earliest.pts.Milliseconds) {
			earliest = &packets[i]
		}
	}
	return earliest
}

func latestPresentedPacket(packets []conditioningPacket, kind StreamKind, index int) (*conditioningPacket, error) {
	var latest *conditioningPacket
	var latestEnd int64
	for i := range packets {
		if packets[i].kind != kind || packets[i].index != index {
			continue
		}
		if !packets[i].duration.Available {
			return nil, nil
		}
		end, err := checkedConditioningAdd(packets[i].pts.Milliseconds, packets[i].duration.Milliseconds)
		if err != nil {
			return nil, err
		}
		if latest == nil || end > latestEnd {
			latest = &packets[i]
			latestEnd = end
		}
	}
	return latest, nil
}

func uniquePacketWithHash(packets []conditioningPacket, kind StreamKind, index int, hash string) *conditioningPacket {
	var matched *conditioningPacket
	for i := range packets {
		if packets[i].kind != kind || packets[i].index != index || packets[i].hash != hash {
			continue
		}
		if matched != nil {
			return nil
		}
		matched = &packets[i]
	}
	return matched
}
