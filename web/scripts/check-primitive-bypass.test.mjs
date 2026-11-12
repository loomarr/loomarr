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
