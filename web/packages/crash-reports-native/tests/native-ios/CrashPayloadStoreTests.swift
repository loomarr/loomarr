import Foundation

// Compiled with ios/CrashPayloadStore.swift by tests/ios-crash-payload-store.test.mjs on macOS.
@main
struct CrashPayloadStoreTests {
  static func check(_ condition: Bool, _ message: String) {
    if !condition {
      FileHandle.standardError.write(Data("FAIL: \(message)\n".utf8))
      exit(1)
    }
  }

  static func main() throws {
    let directory = FileManager.default.temporaryDirectory
      .appendingPathComponent("loomarr-crash-store-\(UUID().uuidString)", isDirectory: true)
    defer { try? FileManager.default.removeItem(at: directory) }
    let store = CrashPayloadStore(directory: directory)
    let payload = Data(#"{"crashDiagnostics":[{"version":"1.0.0"}],"timeStampEnd":"2026-10-13 11:59:00"}"#.utf8)
    let ended = Date(timeIntervalSince1970: 1_760_356_740)

    check(store.pending().isEmpty, "a missing directory has nothing pending")

    let first = try store.save(payload: payload, endedAt: ended, now: Date(timeIntervalSince1970: 1))
    let pending = store.pending()
    check(pending.count == 1, "one payload is pending")
    check(pending.first?["id"] == first, "the envelope names its own id")
    check(pending.first?["endedAt"] == "2025-10-13T11:59:00Z", "the window end is RFC 3339")
    check(pending.first?["payload"] == String(decoding: payload, as: UTF8.self), "the payload is verbatim")

    for second in 2...25 {
      try store.save(payload: payload, endedAt: ended, now: Date(timeIntervalSince1970: Double(second)))
    }
    let bounded = store.pending()
    check(bounded.count == CrashPayloadStore.limit, "only the newest \(CrashPayloadStore.limit) are kept")
    check(!bounded.contains { $0["id"] == first }, "the oldest payload went first")

    store.remove(id: "../escape")
    store.remove(id: bounded[0]["id"] ?? "")
    check(store.pending().count == CrashPayloadStore.limit - 1, "remove deletes exactly the named payload")

    try Data("{torn".utf8).write(to: directory.appendingPathComponent("000000000000001-torn.json"))
    check(store.pending().count == CrashPayloadStore.limit - 1, "a torn file is not returned")
    check(
      !FileManager.default.fileExists(atPath: directory.appendingPathComponent("000000000000001-torn.json").path),
      "a torn file is removed"
    )

    print("ios crash payload store: ok")
  }
}
