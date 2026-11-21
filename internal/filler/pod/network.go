package pod

import "github.com/loomarr/loomarr/internal/channels/schedule"

// networkEligible reports whether c may air on a channel of network (#4432). A clip that presents a
// broadcast network — its ID, bumper or promo, the network a clip's Network names — airs only on a
// channel whose lineup mostly aired on that network; a channel with no network airs no network's.
// A clip with no network is neutral and airs anywhere.
//
// ⚠ A hard boundary like geography, applied before pins and the ladder: a CBS ident on a channel
// of Fox shows is wrong however the break was filled.
func networkEligible(c Clip, network string) bool {
	own := schedule.NetworkKey(c.Network)
	return own == "" || own == schedule.NetworkKey(network)
}

func filterNetwork(clips []Clip, network string) []Clip {
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		if networkEligible(c, network) {
			out = append(out, c)
		}
	}
	return out
}
