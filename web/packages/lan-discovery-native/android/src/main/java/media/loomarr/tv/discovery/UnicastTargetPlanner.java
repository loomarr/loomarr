package media.loomarr.tv.discovery;

import java.net.Inet4Address;
import java.net.InetAddress;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;

/** Plans one bounded IPv4 neighbourhood supplied by Android's selected network. */
final class UnicastTargetPlanner {
  static final int MAX_TARGETS = 254;

  private UnicastTargetPlanner() {}

  static Set<InetAddress> targets(List<Subnet> subnets) throws Exception {
    List<Subnet> ordered = new ArrayList<>(subnets);
    ordered.sort(Comparator.comparingLong(Subnet::network).thenComparingLong(Subnet::local));
    Set<InetAddress> targets = new LinkedHashSet<>();
    Set<Long> localAddresses = new LinkedHashSet<>();
    for (Subnet subnet : ordered) localAddresses.add(subnet.local);
    for (Subnet subnet : ordered) {
      if (targets.size() >= MAX_TARGETS) break;
      addTargets(targets, localAddresses, subnet);
    }
    return targets;
  }

  private static void addTargets(Set<InetAddress> targets, Set<Long> localAddresses, Subnet subnet)
      throws Exception {
    int prefix = Math.max(subnet.prefix, 24);
    if (prefix > 32) return;
    long mask = (0xffff_ffffL << (32 - prefix)) & 0xffff_ffffL;
    long first;
    long last;
    if (prefix == 31) {
      // RFC 3021: both addresses are usable on a point-to-point /31.
      first = subnet.local & mask;
      last = subnet.local | (~mask & 0xffff_ffffL);
    } else if (prefix == 32) {
      // A host route has no neighbour to probe.
      return;
    } else {
      first = (subnet.local & mask) + 1;
      last = (subnet.local | (~mask & 0xffff_ffffL)) - 1;
    }
    for (long candidate = first; candidate <= last && targets.size() < MAX_TARGETS; candidate += 1) {
      if (!localAddresses.contains(candidate)) targets.add(address(candidate));
    }
  }

  private static InetAddress address(long value) throws Exception {
    return InetAddress.getByAddress(new byte[] {
        (byte) (value >>> 24), (byte) (value >>> 16), (byte) (value >>> 8), (byte) value,
    });
  }

  static final class Subnet {
    final long local;
    final int prefix;

    Subnet(InetAddress address, int prefix) {
      if (!(address instanceof Inet4Address)) throw new IllegalArgumentException("IPv4 required");
      byte[] octets = address.getAddress();
      local = ((long) (octets[0] & 0xff) << 24)
          | ((long) (octets[1] & 0xff) << 16)
          | ((long) (octets[2] & 0xff) << 8)
          | (long) (octets[3] & 0xff);
      this.prefix = prefix;
    }

    long network() {
      int effectivePrefix = Math.max(prefix, 24);
      if (effectivePrefix > 32) return local;
      long mask = (0xffff_ffffL << (32 - effectivePrefix)) & 0xffff_ffffL;
      return local & mask;
    }

    long local() {
      return local;
    }
  }
}
