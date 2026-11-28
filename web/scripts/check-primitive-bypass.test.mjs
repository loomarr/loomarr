import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { compareToLedger, measureSource, shrinkLedger } from "./check-primitive-bypass.mjs";

describe("measureSource", () => {
  it("counts each way of sidestepping a primitive", () => {
    const source = `
      import { Modal, View } from "react-native";
      import { Dialog } from "@base-ui/react/dialog";
      import { Menu } from "@radix-ui/react-dropdown-menu";
      export const View1 = () => (
        <div className={cn("flex gap-2", open && "p-1")}>
          <section role="dialog" aria-modal="true" />
          <table />
          <div role="grid" />
          <select />
          <input role="combobox" />
          <span role="tooltip" />
          <ul role="menu" />
        </div>
      );
    `;
    assert.deepEqual(measureSource(source, "apps/web/src/x.tsx"), {
      rawBaseUi: 2,
      rnModal: 1,
      rawDialog: 2,
      rawTable: 2,
      rawCombobox: 2,
      rawTooltip: 1,
      rawMenu: 1,
      flexStack: 1,
    });
  });

  it("ignores type-only imports, other roles and non-flex classes", () => {
    const source = `
      import type { Modal } from "react-native";
      export const Row = () => (
        <div className="grid gap-2" role="listbox">
          <span className="inline-block" role="button" />
        </div>
      );
    `;
    assert.deepEqual(measureSource(source, "apps/web/src/x.tsx"), {});
  });
});

describe("inlineStyle", () => {
  const source = `
    export const Row = ({ open, styles }) => (
      <div style={{ gap: 4 }}>
        <span style={open ? { top: 0 } : undefined} />
        <span style={open && { top: 0 }} />
        <View style={[styles.row, { flex: 1 }]} />
        <span style={styles.row} />
        <span style={style} />
      </div>
    );
  `;

  it("counts object literals handed to style", () => {
    assert.deepEqual(measureSource(source, "apps/web/src/x.tsx"), { inlineStyle: 4 });
  });

  it("exempts the web client-adapters", () => {
    assert.deepEqual(measureSource(source, "apps/web/src/client-adapters/x/x.tsx"), {});
  });

  it("counts a module-level object constant, whole or by entry", () => {
    const named = `
      const ROW = { gap: 4 } as const;
      export const COLUMN: Style = { gap: 4 };
      const styles = { row: { gap: 4 }, label: "x" };
      const LABEL = "x";
      export const Row = ({ open, ROW_PROP }) => (
        <div style={ROW}>
          <span style={COLUMN} />
          <span style={styles.row} />
          <span style={open ? ROW : undefined} />
          <span style={styles.label} />
          <span style={LABEL} />
          <span style={ROW_PROP} />
          <span style={other.row} />
        </div>
      );
    `;
    assert.deepEqual(measureSource(named, "apps/web/src/x.tsx"), { inlineStyle: 4 });
  });

  it("sees through a non-null wrapper", () => {
    const wrapped = `
      const ROW = { gap: 4 };
      const styles = { row: { gap: 4 } };
      export const Row = () => (
        <div style={({ gap: 4 })!}>
          <span style={ROW!} />
          <span style={styles!.row} />
        </div>
      );
    `;
    assert.deepEqual(measureSource(wrapped, "apps/web/src/x.tsx"), { inlineStyle: 3 });
  });

  const count = (body) => measureSource(body, "packages/ui/src/x.tsx").inlineStyle ?? 0;

  it("sees through a cast", () => {
    const cast = `
      const ROW = { gap: 4 } as unknown as ViewStyle;
      export const Row = () => (
        <View style={{ gap: 4 } as ViewStyle}>
          <View style={{ gap: 4 } as const} />
          <View style={ROW} />
        </View>
      );
    `;
    assert.equal(count(cast), 3);
  });

  it("counts a Platform.select, inline or as a constant", () => {
    const selected = `
      const ROW = { gap: 4 };
      const FIXED = Platform.select({ web: { position: "fixed" }, default: undefined });
      const NAMED = Platform.select({ web: ROW }) as ViewStyle;
      const styles = { row: Platform.select({ ios: { gap: 4 } }) };
      const LABEL = Platform.select({ web: "a", default: "b" });
      export const Row = () => (
        <View style={Platform.select({ web: { gap: 4 } })}>
          <View style={FIXED} />
          <View style={NAMED} />
          <View style={styles.row} />
          <View style={LABEL} />
          <View style={Platform.select({ web: undefined })} />
          <View style={other.select({ web: { gap: 4 } })} />
        </View>
      );
    `;
    assert.equal(count(selected), 4);
  });

  it("counts an object behind a Platform.OS condition, inline or as a constant", () => {
    const conditional = `
      const FIXED = Platform.OS === "web" ? ({ position: "fixed" } as object) : undefined;
      const WEB_ONLY = Platform.OS === "web" && { gap: 4 };
      export const Row = () => (
        <View style={Platform.OS === "web" ? ({ gap: 4 } as const) : undefined}>
          <View style={FIXED} />
          <View style={[WEB_ONLY, null]} />
        </View>
      );
    `;
    assert.equal(count(conditional), 3);
  });
});

describe("ledger", () => {
  it("fails on a rise or a new file and shrinks without adding", () => {
    const ledger = { "a.tsx": { rawTable: 2 }, "b.tsx": { flexStack: 1 } };
    const rose = compareToLedger({ "a.tsx": { rawTable: 3 }, "c.tsx": { rawMenu: 1 } }, ledger);
    assert.equal(rose.violations.length, 2);
    const fell = compareToLedger({ "a.tsx": { rawTable: 1 } }, ledger);
    assert.deepEqual(fell, { violations: [], canShrink: true });
    assert.deepEqual(shrinkLedger({ "a.tsx": { rawTable: 1 }, "c.tsx": { rawMenu: 1 } }, ledger), {
      "a.tsx": { rawTable: 1 },
    });
  });
});
