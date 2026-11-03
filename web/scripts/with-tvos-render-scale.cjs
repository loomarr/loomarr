const fs = require("node:fs");
const path = require("node:path");
const { withDangerousMod } = require("@expo/config-plugins");
const { mergeContents } = require("@expo/config-plugins/build/utils/generateCode");

/**
 * Apple TV lays the app out like Android TV (#2141). tvOS reports a 1080p screen as 1920×1080
 * points at 1×, where Android TV reports 960×540 dp at 2×, and every tv-density size is drawn for
 * the latter. The design system's ViewportStage lays the app out at the TV layout width (960) and
 * scales it up to the screen. A scaled layer still rasterizes at its view's contentScaleFactor,
 * which UIKit takes from the screen when the view joins a window, so the scene raises its display
 * scale by the same factor and every view in it draws at that scale: sharp text, not a half-size
 * bitmap stretched to fit.
 */
const RENDER_SCALE = `
  override func scene(
    _ scene: UIScene,
    willConnectTo session: UISceneSession,
    options connectionOptions: UIScene.ConnectionOptions
  ) {
    super.scene(scene, willConnectTo: session, options: connectionOptions)
    #if os(tvOS)
    guard #available(tvOS 17.0, *), let windowScene = scene as? UIWindowScene else { return }
    _ = UIView.loomarrRenderScaleInstalled
    // The design system's TV layout width: keep in step with ViewportStage.
    let stageScale = windowScene.screen.bounds.width / 960
    for window in windowScene.windows {
      window.traitOverrides.displayScale = windowScene.screen.scale * stageScale
      window.loomarrRaiseRenderScale()
    }
    #endif
  }
}

extension UIView {
  /// Every view that joins a window draws at the window's display scale, not the screen's.
  static let loomarrRenderScaleInstalled: Void = {
    guard
      let original = class_getInstanceMethod(UIView.self, #selector(UIView.didMoveToWindow)),
      let raised = class_getInstanceMethod(UIView.self, #selector(UIView.loomarr_didMoveToWindow))
    else { return }
    method_exchangeImplementations(original, raised)
  }()

  @objc func loomarr_didMoveToWindow() {
    loomarr_didMoveToWindow()
    loomarrRaiseRenderScale(recursive: false)
  }

  func loomarrRaiseRenderScale(recursive: Bool = true) {
    guard let window else { return }
    let scale = window.traitCollection.displayScale
    if scale > contentScaleFactor { contentScaleFactor = scale }
    if recursive { subviews.forEach { $0.loomarrRaiseRenderScale() } }
  }
}

// Expo's template closes this extension.
extension SceneDelegate {
`;

const CLASS_ANCHOR = /^class SceneDelegate: ExpoAppSceneDelegate \{$/m;

function raiseRenderScale(source) {
  if (!CLASS_ANCHOR.test(source)) {
    throw new Error("Could not find Expo's SceneDelegate shape while applying the tvOS render scale");
  }
  return mergeContents({
    anchor: CLASS_ANCHOR,
    comment: "//",
    newSrc: RENDER_SCALE,
    offset: 1,
    src: source,
    tag: "loomarr-tvos-render-scale",
  }).contents;
}

function withTvosRenderScale(config) {
  return withDangerousMod(config, [
    "ios",
    (modConfig) => {
      const file = path.join(
        modConfig.modRequest.platformProjectRoot,
        modConfig.modRequest.projectName,
        "SceneDelegate.swift",
      );
      fs.writeFileSync(file, raiseRenderScale(fs.readFileSync(file, "utf8")));
      return modConfig;
    },
  ]);
}

module.exports = withTvosRenderScale;
module.exports.raiseRenderScale = raiseRenderScale;
