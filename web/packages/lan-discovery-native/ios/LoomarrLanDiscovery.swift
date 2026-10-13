import Foundation
import Network
import React

/// iPhone half of design "Finding a server": a Bonjour browse of `_loomarr._tcp` plus two bounded
/// unicast sweeps of the device's own /24 for the UDP responder. iOS sends no broadcast: that needs
/// the multicast entitlement Apple must approve, while browse and unicast need only Local Network.
@objc(LoomarrLanDiscovery)
final class LoomarrLanDiscovery: RCTEventEmitter {
  private static let serviceType = "_loomarr._tcp"
  private static let request = Array("LOOMARR_DISCOVER/1".utf8)
  private static let responderPort: UInt16 = 51029
  private static let sweepIntervalMs: Int64 = 15_000
  private static let networkRecheckIntervalMs: Int64 = 1_000
  private static let packetGapMs = 5

  private let queue = DispatchQueue(label: "media.loomarr.lan-discovery")
  private let lock = NSLock()
  private var generation = 0
  private var hasListeners = false
  private var browser: NWBrowser?
  private var resolving: [ObjectIdentifier: NWConnection] = [:]

  override static func requiresMainQueueSetup() -> Bool { false }

  override func supportedEvents() -> [String]! {
    ["loomarrDiscoveryFound", "loomarrDiscoveryLost", "loomarrDiscoveryError"]
  }

  override func startObserving() { hasListeners = true }
  override func stopObserving() { hasListeners = false }

  override func invalidate() {
    stop()
    super.invalidate()
  }

  @objc func start() {
    stop()
    let active = advanceGeneration()
    queue.async { self.startBrowse(active) }
    let worker = Thread { [weak self] in self?.sweep(active) }
    worker.name = "loomarr-lan-discovery"
    worker.start()
  }

  @objc func stop() {
    _ = advanceGeneration()
    queue.async {
      self.browser?.cancel()
      self.browser = nil
      for connection in self.resolving.values { connection.cancel() }
      self.resolving.removeAll()
    }
  }

  private func advanceGeneration() -> Int {
    lock.lock()
    defer { lock.unlock() }
    generation += 1
    return generation
  }

  private func isActive(_ active: Int) -> Bool {
    lock.lock()
    defer { lock.unlock() }
    return generation == active
  }

  // MARK: Bonjour

  private func startBrowse(_ active: Int) {
    guard isActive(active) else { return }
    let parameters = NWParameters()
    parameters.includePeerToPeer = false
    let browser = NWBrowser(
      for: .bonjourWithTXTRecord(type: Self.serviceType, domain: nil), using: parameters)
    // A denied Local Network permission or a failed browse leaves the unicast sweep running, as a
    // failed NsdManager start does on Android; the 30 s visit then ends in manual entry.
    browser.browseResultsChangedHandler = { [weak self] _, changes in
      guard let self, self.isActive(active) else { return }
      for change in changes {
        switch change {
        case let .added(result):
          self.resolve(result, active)
        case let .changed(_, result, _):
          self.resolve(result, active)
        case let .removed(result):
          if case let .service(name, _, _, _) = result.endpoint {
            self.emit("loomarrDiscoveryLost", ["id": name])
          }
        default:
          break
        }
      }
    }
    self.browser = browser
    browser.start(queue: queue)
  }

  /// NWBrowser reports a service name only. A short TCP connection resolves it to the address the
  /// server listens on; IPv4 is required because a link-local IPv6 URL is useless to the shell.
  private func resolve(_ result: NWBrowser.Result, _ active: Int) {
    guard case let .service(name, _, _, _) = result.endpoint else { return }
    var scheme = "http"
    var serviceProtocol = ""
    if case let .bonjour(record) = result.metadata {
      scheme = record["scheme"] ?? scheme
      serviceProtocol = record["protocol"] ?? ""
    }
    guard scheme == "http" || scheme == "https" else { return }
    let parameters = NWParameters.tcp
    if let ip = parameters.defaultProtocolStack.internetProtocol as? NWProtocolIP.Options {
      ip.version = .v4
    }
    let connection = NWConnection(to: result.endpoint, using: parameters)
    let key = ObjectIdentifier(connection)
    resolving[key] = connection
    connection.stateUpdateHandler = { [weak self, weak connection] state in
      guard let self, let connection else { return }
      switch state {
      case .ready:
        if self.isActive(active),
          case let .hostPort(host, port)? = connection.currentPath?.remoteEndpoint,
          let address = Self.urlHost(host)
        {
          self.emit(
            "loomarrDiscoveryFound",
            [
              "id": name, "name": name, "protocol": serviceProtocol,
              "url": "\(scheme)://\(address):\(port.rawValue)",
            ])
        }
        connection.cancel()
      case .waiting, .failed:
        connection.cancel()
      case .cancelled:
        self.resolving.removeValue(forKey: key)
      default:
        break
      }
    }
    connection.start(queue: queue)
  }

  private static func urlHost(_ host: NWEndpoint.Host) -> String? {
    switch host {
    case let .ipv4(address):
      return "\(address)".split(separator: "%").first.map(String.init)
    case let .ipv6(address):
      return "[\("\(address)".split(separator: "%").first ?? "")]"
    case let .name(name, _):
      return name.isEmpty ? nil : name
    @unknown default:
      return nil
    }
  }

  // MARK: Unicast sweep

  private func sweep(_ active: Int) {
    let socketFD = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP)
    guard socketFD >= 0 else { return }
    defer { close(socketFD) }
    // The one-second receive timeout bounds how long a stopped visit keeps its worker.
    var timeout = timeval(tv_sec: 1, tv_usec: 0)
    setsockopt(socketFD, SOL_SOCKET, SO_RCVTIMEO, &timeout, socklen_t(MemoryLayout<timeval>.size))
    var retry = UnicastRetryPolicy(
      sweepIntervalMs: Self.sweepIntervalMs, recheckIntervalMs: Self.networkRecheckIntervalMs)
    var buffer = [UInt8](repeating: 0, count: 1025)
    while isActive(active) {
      let now = Int64(Date().timeIntervalSince1970 * 1000)
      if retry.isDue(now) {
        let targets = UnicastTargetPlanner.targets(Self.localSubnets())
        if targets.isEmpty {
          // Do not spend either sweep before the device has a LAN address.
          retry.unavailable(now)
        } else {
          UnicastSweep.send(
            targets,
            packetGapMs: Self.packetGapMs,
            isActive: { self.isActive(active) },
            sender: { Self.sendProbe(socketFD, to: $0) },
            sleeper: { usleep(useconds_t($0 * 1000)) })
          retry.sent(now)
        }
      }
      let count = recv(socketFD, &buffer, buffer.count, 0)
      if count > 0, count <= 1024, isActive(active) {
        emitResponse(Data(buffer[0..<count]))
      }
    }
  }

  private static func sendProbe(_ socketFD: Int32, to target: UInt32) {
    var address = sockaddr_in()
    address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
    address.sin_family = sa_family_t(AF_INET)
    address.sin_port = responderPort.bigEndian
    address.sin_addr = in_addr(s_addr: target.bigEndian)
    _ = request.withUnsafeBytes { bytes in
      withUnsafePointer(to: &address) {
        $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
          sendto(
            socketFD, bytes.baseAddress, bytes.count, 0, $0,
            socklen_t(MemoryLayout<sockaddr_in>.size))
        }
      }
    }
  }

  /// IPv4 addresses on Wi-Fi/Ethernet (`en*`): never cellular, VPN tunnels, AWDL or loopback, and
  /// never a self-assigned 169.254 address that has no neighbourhood to sweep.
  private static func localSubnets() -> [UnicastTargetPlanner.Subnet] {
    var head: UnsafeMutablePointer<ifaddrs>?
    guard getifaddrs(&head) == 0, let first = head else { return [] }
    defer { freeifaddrs(head) }
    var subnets: [UnicastTargetPlanner.Subnet] = []
    for pointer in sequence(first: first, next: { $0.pointee.ifa_next }) {
      let entry = pointer.pointee
      let flags = Int32(entry.ifa_flags)
      guard flags & IFF_UP != 0, flags & IFF_RUNNING != 0, flags & IFF_LOOPBACK == 0,
        String(cString: entry.ifa_name).hasPrefix("en"),
        let address = entry.ifa_addr, address.pointee.sa_family == UInt8(AF_INET),
        let netmask = entry.ifa_netmask
      else { continue }
      let local = ipv4(address)
      if local >> 16 == 0xA9FE { continue }
      subnets.append(.init(local: local, prefix: ipv4(netmask).nonzeroBitCount))
    }
    return subnets
  }

  private static func ipv4(_ address: UnsafeMutablePointer<sockaddr>) -> UInt32 {
    address.withMemoryRebound(to: sockaddr_in.self, capacity: 1) {
      UInt32(bigEndian: $0.pointee.sin_addr.s_addr)
    }
  }

  /// Untrusted LAN datagrams that do not match the bounded protocol are ignored.
  private func emitResponse(_ data: Data) {
    guard
      let response = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
      response["protocol"] as? Int == 1,
      let id = response["id"] as? String, !id.isEmpty,
      let name = response["name"] as? String, !name.isEmpty,
      let url = response["url"] as? String,
      let parsed = URLComponents(string: url),
      parsed.scheme == "http" || parsed.scheme == "https",
      let host = parsed.host, !host.isEmpty
    else { return }
    emit("loomarrDiscoveryFound", ["id": id, "name": name, "protocol": "1", "url": url])
  }

  private func emit(_ event: String, _ body: [String: String]) {
    guard hasListeners else { return }
    sendEvent(withName: event, body: body)
  }
}
