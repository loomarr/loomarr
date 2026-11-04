const assert = require("node:assert/strict");
const test = require("node:test");
const { raiseRenderScale } = require("./with-tvos-render-scale.cjs");

const SCENE_DELEGATE = `internal import Expo

@objc(SceneDelegate)
class SceneDelegate: ExpoAppSceneDelegate {
  // Extension point for config plugins.
}
`;

test("raises the scene's display scale by the TV stage factor on tvOS", () => {
  const source = raiseRenderScale(SCENE_DELEGATE);

  assert.match(source, /super\.scene\(scene, willConnectTo: session, options: connectionOptions\)/);
  assert.match(source, /#if os\(tvOS\)/);
  assert.match(source, /let stageScale = windowScene\.screen\.bounds\.width \/ 960/);
  assert.match(source, /window\.traitOverrides\.displayScale = windowScene\.screen\.scale \* stageScale/);
  assert.match(source, /method_exchangeImplementations\(original, raised\)/);
});

test("leaves the template's closing brace a whole declaration to close", () => {
  const source = raiseRenderScale(SCENE_DELEGATE);
  const opens = (source.match(/\{/g) ?? []).length;
  const closes = (source.match(/\}/g) ?? []).length;

  assert.equal(opens, closes);
  assert.match(source, /extension SceneDelegate \{\n[^{}]*\/\/ Extension point for config plugins\.\n\}\n$/);
});

test("applies once, however often prebuild runs", () => {
  const once = raiseRenderScale(SCENE_DELEGATE);

  assert.equal(raiseRenderScale(once), once);
});

test("fails closed when Expo's SceneDelegate shape changes", () => {
  assert.throws(() => raiseRenderScale("class Other {}\n"), /SceneDelegate/);
});
