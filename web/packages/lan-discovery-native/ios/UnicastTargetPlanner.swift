/// Plans one bounded IPv4 neighbourhood from the device's own Wi-Fi/Ethernet addresses. It mirrors
/// the Android planner: at most its own /24 per address, the local addresses excluded, 254 targets.
enum UnicastTargetPlanner {
  static let maxTargets = 254

  struct Subnet {
    let local: UInt32
    let prefix: Int

    var network: UInt32 {
      let effectivePrefix = max(prefix, 24)
      return effectivePrefix > 32 ? local : local & UnicastTargetPlanner.mask(effectivePrefix)
    }
  }

  static func targets(_ subnets: [Subnet]) -> [UInt32] {
    let ordered = subnets.sorted { ($0.network, $0.local) < ($1.network, $1.local) }
    let localAddresses = Set(ordered.map(\.local))
    var targets: [UInt32] = []
    var planned = Set<UInt32>()
    for subnet in ordered where targets.count < maxTargets {
      let prefix = max(subnet.prefix, 24)
      // A host route (or a malformed prefix) has no neighbour to probe.
      if prefix >= 32 { continue }
      let mask = mask(prefix)
      let first: UInt32
      let last: UInt32
      if prefix == 31 {
        // RFC 3021: both addresses are usable on a point-to-point /31.
        first = subnet.local & mask
        last = subnet.local | ~mask
      } else {
        first = (subnet.local & mask) + 1
        last = (subnet.local | ~mask) - 1
      }
      var candidate = first
      while candidate <= last, targets.count < maxTargets {
        if !localAddresses.contains(candidate), planned.insert(candidate).inserted {
          targets.append(candidate)
        }
        if candidate == last { break }
        candidate += 1
      }
    }
    return targets
  }

  static func dottedQuad(_ address: UInt32) -> String {
    "\(address >> 24 & 0xff).\(address >> 16 & 0xff).\(address >> 8 & 0xff).\(address & 0xff)"
  }

  private static func mask(_ prefix: Int) -> UInt32 {
    prefix <= 0 ? 0 : UInt32.max << UInt32(32 - prefix)
  }
}

/// Tracks two foreground unicast sweeps without spending one before a LAN address exists.
struct UnicastRetryPolicy {
  private let sweepIntervalMs: Int64
  private let recheckIntervalMs: Int64
  private(set) var remaining = 2
  private var nextAttemptMs: Int64 = 0

  init(sweepIntervalMs: Int64, recheckIntervalMs: Int64) {
    self.sweepIntervalMs = sweepIntervalMs
    self.recheckIntervalMs = recheckIntervalMs
  }

  func isDue(_ nowMs: Int64) -> Bool {
    remaining > 0 && nowMs >= nextAttemptMs
  }

  mutating func unavailable(_ nowMs: Int64) {
    nextAttemptMs = nowMs + recheckIntervalMs
  }

  mutating func sent(_ nowMs: Int64) {
    remaining -= 1
    nextAttemptMs = nowMs + sweepIntervalMs
  }
}

/// Paces one sweep and stops between packets as soon as discovery is cancelled.
enum UnicastSweep {
  @discardableResult
  static func send(
    _ targets: [UInt32],
    packetGapMs: Int,
    isActive: () -> Bool,
    sender: (UInt32) -> Void,
    sleeper: (Int) -> Void
  ) -> Bool {
    for target in targets {
      if !isActive() { return false }
      // A failed send to one unreachable neighbour must not stop the bounded sweep.
      sender(target)
      if packetGapMs > 0 {
        sleeper(packetGapMs)
        if !isActive() { return false }
      }
    }
    return isActive()
  }
}
