// Executable coverage for the iOS planner, retry policy and paced sweep (compiled with the
// production ios/UnicastTargetPlanner.swift by tests/ios-unicast-target-planner.test.mjs).
import Foundation

@main
enum UnicastTargetPlannerTests {
  static func main() {
    isOrderIndependentAndPlansTheLocalNeighbourhood()
    excludesTheLocalAddressesAndDeduplicates()
    keepsThePeerFor31AndNoTargetsFor32()
    capsEverySweepAt254Targets()
    clampsAWiderPrefixToItsOwn24()
    retainsBothSweepsUntilALanAddressArrives()
    stopsSendingWhenCancelledDuringPacing()
    print("ios unicast planner: ok")
  }

  private static func isOrderIndependentAndPlansTheLocalNeighbourhood() {
    let lan = subnet("192.168.50.10", 24)
    let peer = subnet("192.168.50.20", 24)
    let first = UnicastTargetPlanner.targets([lan, peer])
    check(first == UnicastTargetPlanner.targets([peer, lan]), "target order must be deterministic")
    check(first.first == address("192.168.50.1"), "the sweep starts at the first host")
    check(!first.contains(address("192.168.50.255")), "the broadcast address is never probed")
  }

  private static func excludesTheLocalAddressesAndDeduplicates() {
    let targets = UnicastTargetPlanner.targets([subnet("192.168.1.10", 24), subnet("192.168.1.11", 24)])
    check(!targets.contains(address("192.168.1.10")), "local address must be excluded")
    check(targets.count == 252, "both local addresses excluded and targets deduplicated")
  }

  private static func keepsThePeerFor31AndNoTargetsFor32() {
    check(
      UnicastTargetPlanner.targets([subnet("192.168.1.0", 31)]) == [address("192.168.1.1")],
      "/31 must probe its one peer")
    check(UnicastTargetPlanner.targets([subnet("192.168.1.1", 32)]).isEmpty, "/32 has no peer")
  }

  private static func capsEverySweepAt254Targets() {
    let targets = UnicastTargetPlanner.targets([subnet("10.0.0.10", 24), subnet("10.0.1.10", 24)])
    check(targets.count == 254, "two LANs must remain capped at 254")
  }

  private static func clampsAWiderPrefixToItsOwn24() {
    let targets = UnicastTargetPlanner.targets([subnet("10.1.2.3", 16)])
    check(targets.count == 253, "a /16 sweeps only the device's own /24")
    check(targets.allSatisfy { $0 >> 8 == address("10.1.2.0") >> 8 }, "targets stay in the own /24")
  }

  private static func retainsBothSweepsUntilALanAddressArrives() {
    var retry = UnicastRetryPolicy(sweepIntervalMs: 15_000, recheckIntervalMs: 1_000)
    check(retry.isDue(0), "first sweep must be immediately eligible")
    retry.unavailable(0)
    check(retry.remaining == 2 && !retry.isDue(999), "a late address must preserve both sweeps")
    check(retry.isDue(1_000), "a late address must be retried")
    retry.sent(1_000)
    check(retry.remaining == 1 && !retry.isDue(15_999), "the first send schedules the second sweep")
    check(retry.isDue(16_000), "the second sweep remains due")
    retry.sent(16_000)
    check(!retry.isDue(1_000_000), "there is no third sweep")
  }

  private static func stopsSendingWhenCancelledDuringPacing() {
    var sent = 0
    var active = true
    var slept: [Int] = []
    let complete = UnicastSweep.send(
      [address("192.168.1.1"), address("192.168.1.2"), address("192.168.1.3")],
      packetGapMs: 5,
      isActive: { active },
      sender: { _ in sent += 1 },
      sleeper: {
        slept.append($0)
        active = false
      })
    check(!complete && sent == 1 && slept == [5], "cancellation must stop the remaining paced sends")
  }

  private static func subnet(_ dotted: String, _ prefix: Int) -> UnicastTargetPlanner.Subnet {
    UnicastTargetPlanner.Subnet(local: address(dotted), prefix: prefix)
  }

  private static func address(_ dotted: String) -> UInt32 {
    dotted.split(separator: ".").reduce(0) { $0 << 8 | UInt32($1)! }
  }

  private static func check(_ condition: Bool, _ message: String) {
    if !condition {
      print("FAIL: \(message)")
      exit(1)
    }
  }
}
