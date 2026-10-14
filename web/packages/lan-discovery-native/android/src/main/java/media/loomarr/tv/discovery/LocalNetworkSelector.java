package media.loomarr.tv.discovery;

import java.util.List;

/** Chooses one public-API local transport without inferring private VPN underlay metadata. */
final class LocalNetworkSelector {
  static final class Candidate<T> {
    final T network;
    final boolean isDefault;
    final boolean isVpn;
    final boolean isLocalTransport;
    final boolean matchesActiveVpnType;

    Candidate(T network, boolean isDefault, boolean isVpn, boolean isLocalTransport, boolean matchesActiveVpnType) {
      this.network = network;
      this.isDefault = isDefault;
      this.isVpn = isVpn;
      this.isLocalTransport = isLocalTransport;
      this.matchesActiveVpnType = matchesActiveVpnType;
    }
  }

  private LocalNetworkSelector() {}

  static <T> T select(List<Candidate<T>> candidates) {
    for (Candidate<T> candidate : candidates) {
      if (candidate.isDefault && !candidate.isVpn && candidate.isLocalTransport) return candidate.network;
    }
    T underlying = null;
    for (Candidate<T> candidate : candidates) {
      if (!candidate.matchesActiveVpnType || candidate.isVpn || !candidate.isLocalTransport) continue;
      if (underlying != null) return null; // Two matching LANs are ambiguous; do not guess.
      underlying = candidate.network;
    }
    return underlying;
  }
}
