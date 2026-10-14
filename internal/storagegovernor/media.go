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

// EstimateDiagnosticOutput reserves the atomic-rewrite peak for one bounded process log: the
// current retained file and its replacement temp can coexist during a flush. The fixed overhead
// covers timestamps, the discard marker, and filesystem metadata.
func EstimateDiagnosticOutput(prefixBytes, tailBytes int) (int64, bool) {
	if prefixBytes <= 0 || tailBytes <= 0 {
		return 0, false
	}
	const overhead = int64(64 << 10)
	retained := saturatingAdd(int64(prefixBytes), int64(tailBytes))
	reservation := saturatingAdd(saturatingMultiply(retained, 2), overhead)
	if reservation <= retained {
		return 0, false
	}
	return reservation, true
}

// UnknownAcquisitionCeilingBytes is the staging cap for an item whose provider reports neither a
// byte count nor a duration. Filler clips are short commercials and bumpers, so 512 MiB is far
// above any legitimate one; anything larger is aborted mid-download by the write guard rather
// than refused up front. Refusing was the old behaviour and stalled a whole source (#1394).
const UnknownAcquisitionCeilingBytes = 512 << 20

// EstimateAcquisition budgets ONE acquisition (download into private staging). The acquisition
// lease only has to cover the bytes that can exist on disk while the download runs, and the write
// guard aborts anything past WriteCeilingBytes, so the reservation is exactly the ceiling:
//
//	ceiling     = source + max(source/4, 32 MiB)   (25% margin for sidecars and container overhead)
//	reservation = ceiling
//
// Later stages (transcode, split, artwork) each reserve their own peak through
// Reserve when they run, so this lease deliberately does not pre-reserve their derivatives. The
// old shared formula (ceiling×4 + 64 MiB, ~5× the source) held budget for work that had not
// started and made auto-fetch pause on library_limit long before the disk was actually full.
func EstimateAcquisition(estimate MediaEstimate) (MediaBudget, bool) {
	budget, ok := EstimateMedia(estimate)
	if !ok {
		return MediaBudget{}, false
	}
	return MediaBudget{WriteCeilingBytes: budget.WriteCeilingBytes, ReservationBytes: budget.WriteCeilingBytes}, true
}

// UnknownAcquisitionBudget is the bounded fallback when nothing about the item's size is known.
func UnknownAcquisitionBudget() MediaBudget {
	return MediaBudget{WriteCeilingBytes: UnknownAcquisitionCeilingBytes, ReservationBytes: UnknownAcquisitionCeilingBytes}
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
