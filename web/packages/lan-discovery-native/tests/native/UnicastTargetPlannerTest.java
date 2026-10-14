package media.loomarr.tv.discovery;

import java.net.InetAddress;
import java.util.Arrays;
import java.util.LinkedHashSet;
import java.util.Set;

public final class UnicastTargetPlannerTest {
  public static void main(String[] args) throws Exception {
    choosesOnlyTheSelectedNetworkAndIsOrderIndependent();
    selectsTheVpnUnderlyingLanWithoutGuessingBetweenLans();
    excludesTheLocalAddressAndDeduplicates();
    keepsThePeerFor31AndNoTargetsFor32();
    capsEverySweepAt254Targets();
    retainsBothSweepsUntilALanPlanArrives();
    stopsSendingWhenCancelledDuringPacing();
  }

  private static void choosesOnlyTheSelectedNetworkAndIsOrderIndependent() throws Exception {
    UnicastTargetPlanner.Subnet lan = subnet("192.168.50.10", 24);
    // A VPN/unrelated interface is deliberately absent: Android supplies only its active network.
    Set<InetAddress> first = UnicastTargetPlanner.targets(Arrays.asList(lan, subnet("192.168.50.20", 24)));
    Set<InetAddress> second = UnicastTargetPlanner.targets(Arrays.asList(subnet("192.168.50.20", 24), lan));
    check(first.equals(second), "active-network target order must be deterministic");
    check(first.contains(InetAddress.getByName("192.168.50.1")), "selected LAN must be planned");
    check(!first.contains(InetAddress.getByName("10.8.0.1")), "unrelated interfaces cannot consume the cap");
  }

  private static void excludesTheLocalAddressAndDeduplicates() throws Exception {
    Set<InetAddress> targets = UnicastTargetPlanner.targets(Arrays.asList(
        subnet("192.168.1.10", 24), subnet("192.168.1.11", 24)));
    check(!targets.contains(InetAddress.getByName("192.168.1.10")), "local address must be excluded");
    check(targets.size() == 252, "both local addresses must be excluded and targets deduplicated");
  }

  private static void selectsTheVpnUnderlyingLanWithoutGuessingBetweenLans() {
    String selected = LocalNetworkSelector.select(Arrays.asList(
        new LocalNetworkSelector.Candidate<>("vpn", true, true, false, false),
        new LocalNetworkSelector.Candidate<>("wifi", false, false, true, true),
        new LocalNetworkSelector.Candidate<>("ethernet", false, false, true, false)));
    check("wifi".equals(selected), "VPN default must use its platform-reported Wi-Fi underlay");
    String ambiguous = LocalNetworkSelector.select(Arrays.asList(
        new LocalNetworkSelector.Candidate<>("vpn", true, true, false, false),
        new LocalNetworkSelector.Candidate<>("wifi", false, false, true, true),
        new LocalNetworkSelector.Candidate<>("ethernet", false, false, true, true)));
    check(ambiguous == null, "multiple VPN underlays must not be selected by enumeration order");
  }

  private static void keepsThePeerFor31AndNoTargetsFor32() throws Exception {
    Set<InetAddress> pointToPoint = UnicastTargetPlanner.targets(Arrays.asList(subnet("192.168.1.0", 31)));
    check(pointToPoint.equals(setOf("192.168.1.1")), "/31 must probe its one peer");
    check(UnicastTargetPlanner.targets(Arrays.asList(subnet("192.168.1.1", 32))).isEmpty(), "/32 has no peer");
  }

  private static void capsEverySweepAt254Targets() throws Exception {
    Set<InetAddress> targets = UnicastTargetPlanner.targets(Arrays.asList(
        subnet("10.0.0.10", 24), subnet("10.0.1.10", 24)));
    check(targets.size() == 254, "large active LANs must remain capped");
  }

  private static void retainsBothSweepsUntilALanPlanArrives() {
    UnicastRetryPolicy retry = new UnicastRetryPolicy(15_000, 1_000);
    check(retry.isDue(0), "first sweep must be immediately eligible");
    retry.unavailable(0);
    check(retry.remaining() == 2 && !retry.isDue(999), "late address must preserve both sweeps");
    check(retry.isDue(1_000), "late address must be retried");
    retry.sent(1_000);
    check(retry.remaining() == 1 && !retry.isDue(15_999), "first send schedules the second sweep");
    check(retry.isDue(16_000), "second sweep remains executable after a late address");
  }

  private static void stopsSendingWhenCancelledDuringPacing() throws Exception {
    Set<InetAddress> targets = setOf("192.168.1.1", "192.168.1.2", "192.168.1.3");
    int[] sent = {0};
    boolean[] active = {true};
    boolean complete = UnicastSweep.send(targets, 5, () -> active[0], target -> sent[0] += 1, milliseconds -> active[0] = false);
    check(!complete && sent[0] == 1, "cancellation must stop remaining paced sends");
  }

  private static UnicastTargetPlanner.Subnet subnet(String address, int prefix) throws Exception {
    return new UnicastTargetPlanner.Subnet(InetAddress.getByName(address), prefix);
  }

  private static Set<InetAddress> setOf(String... addresses) throws Exception {
    Set<InetAddress> result = new LinkedHashSet<>();
    for (String address : addresses) result.add(InetAddress.getByName(address));
    return result;
  }

  private static void check(boolean condition, String message) {
    if (!condition) throw new AssertionError(message);
  }
}
