package storagegovernor

const (
	mediaEstimateFloor        = 32 << 20
	mediaEstimateMarginDenom  = 4
	mediaDerivativeMultiplier = 4
	mediaDerivativeFloor      = 64 << 20
	artworkReservationBytes   = 64 << 20
)

// MediaEstimate is provider metadata, not trusted capacity. DeclaredBytes may
// be exact or approximate. DurationMS and Height let the governor produce a
// conservative fallback when the provider omits a byte count.
type MediaEstimate struct {
	DeclaredBytes int64
	DurationMS    int64
	Height        int
}

// EstimateArtwork returns the bounded peak allowance for one still plus one animated preview.
// Artwork is deliberately independent of source size: both outputs use a fixed preview window
// and width, while the generous ceiling also covers the larger GIF compatibility fallback.
func EstimateArtwork() int64 {
	return artworkReservationBytes
}

// EstimatePrepared translates an exact scheduled duration and rendition bitrate into the peak
// allowance for one immutable prepared publication. The fixed floor covers playlists, init
// segments, metadata, and bitrate variation without teaching the prepared-media package storage
// arithmetic.
func EstimatePrepared(durationMS int64, videoBitrateKbps, audioBitrateKbps int) (int64, bool) {
	if durationMS <= 0 || videoBitrateKbps <= 0 || audioBitrateKbps < 0 {
		return 0, false
	}
	bitrate := saturatingAdd(int64(videoBitrateKbps), int64(audioBitrateKbps))
	const maximumInt64 = int64(^uint64(0) >> 1)
	if bitrate <= 0 || durationMS > maximumInt64/bitrate {
		return 0, false
	}
	// milliseconds × kilobits/second ÷ 8 is bytes: both SI factors are 1,000.
	bytes := durationMS * bitrate / 8
	if bytes <= 0 {
		return 0, false
	}
	reservation := saturatingAdd(bytes, max(bytes/mediaEstimateMarginDenom, mediaDerivativeFloor))
	if reservation <= bytes {
		return 0, false
	}
	return reservation, true
}

// MediaBudget is the governor-owned translation from provider facts to limits.
// WriteCeilingBytes bounds acquisition staging. ReservationBytes also accounts
// for the retained source, evidence/playback derivatives, and small generated
// assets that can coexist during later processing.
type MediaBudget struct {
	WriteCeilingBytes int64
	ReservationBytes  int64
}

// EstimateMedia returns a conservative media budget. It refuses metadata that
// cannot establish either a positive byte count or a positive duration.
func EstimateMedia(estimate MediaEstimate) (MediaBudget, bool) {
	sourceBytes := estimate.DeclaredBytes
	if sourceBytes <= 0 {
		bitrate := estimatedBitrate(estimate.Height)
		if estimate.DurationMS <= 0 || bitrate <= 0 {
			return MediaBudget{}, false
		}
		// bitrate is bits/second and duration is milliseconds. Divide by 8,000
		// to produce bytes without converting either input through float64.
		sourceBytes = saturatingMultiply(estimate.DurationMS, bitrate) / 8_000
	}
	if sourceBytes <= 0 {
		return MediaBudget{}, false
	}

	margin := sourceBytes / mediaEstimateMarginDenom
	if margin < mediaEstimateFloor {
		margin = mediaEstimateFloor
	}
	writeCeiling := saturatingAdd(sourceBytes, margin)
	reservation := saturatingAdd(saturatingMultiply(writeCeiling, mediaDerivativeMultiplier), mediaDerivativeFloor)
	if writeCeiling <= sourceBytes || reservation <= writeCeiling {
		return MediaBudget{}, false
	}
	return MediaBudget{WriteCeilingBytes: writeCeiling, ReservationBytes: reservation}, true
}

func estimatedBitrate(height int) int64 {
	switch {
	case height <= 0:
		// Unknown resolution still gets a bounded 1080p-class estimate.
		return 20_000_000
	case height <= 480:
		return 8_000_000
	case height <= 720:
		return 12_000_000
	case height <= 1080:
		return 20_000_000
	default:
		return 50_000_000
	}
}
