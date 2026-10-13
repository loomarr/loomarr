import Foundation

/// MetricKit payloads waiting for JavaScript to take them: one JSON file per payload, holding the
/// payload's own JSON and the end of the window it covers. Plain Foundation, so the Apple CI job can
/// compile and run it without React Native. Uses no required-reason API: it lists names and reads
/// contents, never file timestamps.
struct CrashPayloadStore {
  static let limit = 20

  let directory: URL

  static func applicationSupport() -> URL {
    let root = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
    return root.appendingPathComponent("LoomarrCrashReports/metrickit", isDirectory: true)
  }

  /// Stores one payload and keeps only the newest `limit`. Names sort by arrival, so the oldest go.
  @discardableResult
  func save(payload: Data, endedAt: Date, now: Date = Date()) throws -> String {
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    let id = String(format: "%015lld", Int64(now.timeIntervalSince1970 * 1000)) + "-" + UUID().uuidString
    let envelope: [String: String] = [
      "endedAt": ISO8601DateFormatter().string(from: endedAt),
      "id": id,
      "payload": String(decoding: payload, as: UTF8.self),
    ]
    let data = try JSONSerialization.data(withJSONObject: envelope, options: [.sortedKeys])
    try data.write(to: file(id), options: .atomic)
    for stale in ids().dropLast(Self.limit) { remove(id: stale) }
    return id
  }

  /// Every stored envelope, oldest first. An unreadable file is dropped rather than returned.
  func pending() -> [[String: String]] {
    ids().compactMap { id in
      guard
        let data = try? Data(contentsOf: file(id)),
        let envelope = try? JSONSerialization.jsonObject(with: data) as? [String: String],
        envelope["id"] == id, envelope["endedAt"] != nil, envelope["payload"] != nil
      else {
        remove(id: id)
        return nil
      }
      return envelope
    }
  }

  func remove(id: String) {
    guard Self.isID(id) else { return }
    try? FileManager.default.removeItem(at: file(id))
  }

  private func ids() -> [String] {
    let names = (try? FileManager.default.contentsOfDirectory(atPath: directory.path)) ?? []
    return names
      .filter { $0.hasSuffix(".json") }
      .map { String($0.dropLast(5)) }
      .filter(Self.isID)
      .sorted()
  }

  private func file(_ id: String) -> URL {
    directory.appendingPathComponent(id + ".json", isDirectory: false)
  }

  // JavaScript names the payload it has taken; only a name this store could have made is accepted.
  private static func isID(_ id: String) -> Bool {
    !id.isEmpty && id.allSatisfy { $0.isASCII && ($0.isLetter || $0.isNumber || $0 == "-") }
  }
}
