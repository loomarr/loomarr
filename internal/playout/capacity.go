package playout

// EffectiveCapacity returns the live video-transcode budget for this process. Measurement is the
// authority: a positive operator cap may lower it but cannot claim more capacity than the encoder
// trial proved. An unavailable measurement permits one conservative transcode rather than either
// disabling playout or failing open. Resident model VRAM shades the result last, with the same
// one-transcode floor.
func EffectiveCapacity(measured, cap int, residentVRAMGiB float64) int {
	if measured < 1 {
		measured = 1
	}
	if cap > 0 && cap < measured {
		measured = cap
	}
	if residentVRAMGiB > 0 {
		const vramGiBPerEncode = 4.0
		measured -= int(residentVRAMGiB / vramGiBPerEncode)
		if measured < 1 {
			measured = 1
		}
	}
	return measured
}
