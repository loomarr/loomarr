import assert from "node:assert/strict";
import test from "node:test";

import { assertSurface, surfaceFromXml, verifyTvEmulatorJourney } from "./verify-tv-emulator-journey.mjs";

const labels = {
  guide: "Programme guide",
  surf: "Channel surfer",
  watching: "Open programme guide",
};
const xml = (description) => `<hierarchy><node content-desc="${description}" /></hierarchy>`;

test("classifies only contractual top-level surfaces", () => {
  assert.equal(surfaceFromXml('<node text="PAIRING CODE" />'), "pairing");
  assert.equal(surfaceFromXml(xml(labels.guide)), "guide");
  assert.equal(surfaceFromXml(xml(labels.surf)), "surf");
  assert.equal(surfaceFromXml(xml(labels.watching)), "watching");
  assert.equal(surfaceFromXml(xml("Loading channels")), "unknown");
});

test("rejects leaked Watching chrome behind a sibling journey", () => {
  assert.throws(
    () => assertSurface(`${xml(labels.guide)} ${xml(labels.watching)}`, "guide"),
    /Watching chrome remained accessible behind guide/,
  );
});

test("runs Guide, Surf, Back, and background restoration in order", async () => {
  const surfaces = ["watching", "guide", "watching", "surf", "watching", "watching"];
  const keys = [];
  let starts = 0;
  const result = await verifyTvEmulatorJourney({
    dump: () => xml(labels[surfaces.shift()]),
    key: (key) => keys.push(key),
    model: () => "sdk_gphone_x86_64",
    qemu: () => "1",
    start: () => {
      starts += 1;
    },
  });

  assert.deepEqual(keys, [
    "KEYCODE_DPAD_CENTER",
    "KEYCODE_BACK",
    "KEYCODE_DPAD_LEFT",
    "KEYCODE_BACK",
    "KEYCODE_HOME",
  ]);
  assert.equal(starts, 1);
  assert.equal(result.model, "sdk_gphone_x86_64");
});

test("refuses hardware and unpaired emulators", async () => {
  await assert.rejects(
    verifyTvEmulatorJourney({ model: () => "SHIELD Android TV", qemu: () => "0" }),
    /emulator-only/,
  );
  await assert.rejects(
    verifyTvEmulatorJourney({
      dump: () => '<node text="PAIRING CODE" />',
      model: () => "sdk_gphone_x86_64",
      qemu: () => "1",
    }),
    /not paired/,
  );
});
