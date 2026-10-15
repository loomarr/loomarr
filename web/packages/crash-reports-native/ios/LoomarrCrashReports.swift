import Foundation
import MetricKit
import React

/// Native crashes on the iPhone, through Apple's MetricKit: iOS delivers a crash's diagnostic on the
/// next launch to whoever subscribes. Each payload is persisted before anything else so a crash
/// during delivery loses nothing, then JavaScript takes the pending ones, queues them as reports and
/// removes them here. Nothing leaves the device from this module.
@objc(LoomarrCrashReports)
final class LoomarrCrashReports: RCTEventEmitter, MXMetricManagerSubscriber {
  private let store = CrashPayloadStore(directory: CrashPayloadStore.applicationSupport())
  private let queue = DispatchQueue(label: "media.loomarr.crash-reports")
  private let lock = NSLock()
  private var subscribed = false
  private var hasListeners = false

  override static func requiresMainQueueSetup() -> Bool { false }

  override func supportedEvents() -> [String]! { ["loomarrCrashPayloadsAvailable"] }

  override func startObserving() { setListening(true) }
  override func stopObserving() { setListening(false) }

  override func invalidate() {
    MXMetricManager.shared.remove(self)
    super.invalidate()
  }

  /// This build and device, for reports about JavaScript errors. A native crash carries MetricKit's
  /// record of the build that crashed instead.
  override func constantsToExport() -> [AnyHashable: Any]! {
    let info = Bundle.main.infoDictionary ?? [:]
    let os = ProcessInfo.processInfo.operatingSystemVersion
    return [
      "appBuild": info["CFBundleVersion"] as? String ?? "",
      "appVersion": info["CFBundleShortVersionString"] as? String ?? "",
      "deviceModel": Self.deviceModel(),
      "osVersion": "\(os.majorVersion).\(os.minorVersion).\(os.patchVersion)",
    ]
  }

  @objc func start() {
    lock.lock()
    defer { lock.unlock() }
    guard !subscribed else { return }
    subscribed = true
    MXMetricManager.shared.add(self)
  }

  @objc(pending:rejecter:)
  func pending(_ resolve: @escaping RCTPromiseResolveBlock, rejecter _: @escaping RCTPromiseRejectBlock) {
    queue.async { resolve(self.store.pending()) }
  }

  @objc(remove:resolver:rejecter:)
  func remove(
    _ id: String,
    resolver resolve: @escaping RCTPromiseResolveBlock,
    rejecter _: @escaping RCTPromiseRejectBlock
  ) {
    queue.async {
      self.store.remove(id: id)
      resolve(nil)
    }
  }

  func didReceive(_ payloads: [MXDiagnosticPayload]) {
    queue.sync {
      for payload in payloads {
        try? store.save(payload: payload.jsonRepresentation(), endedAt: payload.timeStampEnd)
      }
    }
    lock.lock()
    let notify = hasListeners
    lock.unlock()
    if notify { sendEvent(withName: "loomarrCrashPayloadsAvailable", body: nil) }
  }

  private func setListening(_ listening: Bool) {
    lock.lock()
    hasListeners = listening
    lock.unlock()
  }

  // The hardware identifier ("iPhone17,1"). The Simulator reports the Mac's architecture, so it
  // names the device it simulates instead.
  private static func deviceModel() -> String {
    if let simulated = ProcessInfo.processInfo.environment["SIMULATOR_MODEL_IDENTIFIER"] { return simulated }
    var system = utsname()
    uname(&system)
    return withUnsafeBytes(of: &system.machine) { bytes in
      String(decoding: bytes.prefix { $0 != 0 }, as: UTF8.self)
    }
  }
}
