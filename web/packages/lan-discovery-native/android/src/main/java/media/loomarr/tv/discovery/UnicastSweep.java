package media.loomarr.tv.discovery;

import java.net.InetAddress;
import java.util.Set;

/** Small cancellation seam shared by UDP sends and executable native unit coverage. */
final class UnicastSweep {
  interface Active { boolean isActive(); }
  interface Sender { void send(InetAddress target) throws Exception; }
  interface Sleeper { void sleep(long milliseconds) throws InterruptedException; }

  private UnicastSweep() {}

  static boolean send(
      Set<InetAddress> targets, long packetGapMs, Active active, Sender sender, Sleeper sleeper)
      throws InterruptedException {
    for (InetAddress target : targets) {
      if (!active.isActive()) return false;
      try {
        sender.send(target);
      } catch (Exception ignored) {
        // One unreachable neighbour must not stop the bounded sweep.
      }
      if (packetGapMs > 0) {
        sleeper.sleep(packetGapMs);
        if (!active.isActive()) return false;
      }
    }
    return active.isActive();
  }
}
