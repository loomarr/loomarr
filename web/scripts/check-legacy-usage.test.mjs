import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import {
  compareRetired,
  compareToLedger,
  measureSource,
  retiredImports,
  shrinkLedger,
} from "./check-legacy-usage.mjs";

const appFile = (path) => fileURLToPath(new URL(`../apps/web/src/${path}`, import.meta.url));

describe("measureSource", () => {
  it("classifies every import form by the legacy module it names", () => {
    const source = `
      import type { VariantProps } from "class-variance-authority";
      import { Dialog } from "@base-ui/react/dialog";
      import { Button } from "@/components/ui/button";
      export * from "@/components/ui";
      const lazy = import("@/components/ui/sheet");
      import { Card } from "../../components/ui/card";
      import { formatTime } from "@/lib/time";
    `;
    assert.deepEqual(measureSource(source, appFile("feature/nested/view.ts")), {
      cva: 1,
      baseUi: 1,
      legacyUi: 4,
    });
  });

  it("does not count a copied component's own wiring as a consumer", () => {
    const own = `export * from "./button"; export type { ButtonProps } from "./button.type";`;
    assert.deepEqual(measureSource(own, appFile("components/ui/button/index.ts")), {});
    const other = `import { Button } from "../button";`;
    assert.deepEqual(measureSource(other, appFile("components/ui/dialog/dialog.tsx")), { legacyUi: 1 });
  });

  it("counts each lucide-react import, however it is written", () => {
    const source = `
      import { Check, X } from "lucide-react";
      import type { LucideIcon } from "lucide-react";
      import { Icon } from "@loomarr/design-system";
      export { Plus } from "lucide-react";
    `;
    assert.deepEqual(measureSource(source, appFile("feature/view.tsx")), { lucideReact: 3 });
  });

  it("counts each raw button element, and not a design-system Action or a prop named button", () => {
    const source = `
      export const Row = ({ button }: { button: string }) => (
        <div>
          <button type="button">Close</button>
          <Action>Open</Action>
          <label data-kind="button" />
          <button type="submit" />
        </div>
      );
    `;
    assert.deepEqual(measureSource(source, appFile("feature/row.tsx")), { rawButton: 2 });
  });

  it("counts legacy component imports in CSS", () => {
    const css = `@import "@loomarr/design-system/theme.css";\n@import "@/components/ui/button/button.css";\n`;
    assert.deepEqual(measureSource(css, appFile("styles.css")), { legacyUi: 1 });
  });
});

describe("compareToLedger", () => {
  const ledger = { "src/a.tsx": { rawButton: 10, cva: 1 } };

  it("fails a rise and a file the ledger does not list", () => {
    const { violations } = compareToLedger(
      { "src/a.tsx": { rawButton: 11, cva: 1 }, "src/b.tsx": { baseUi: 1 } },
      ledger,
    );
    assert.deepEqual(violations, [
      "src/a.tsx: rawButton rose from 10 to 11",
      "src/b.tsx: new file with legacy usage (baseUi 1)",
    ]);
  });

  it("fails a rise in the icon measure", () => {
    const { violations } = compareToLedger(
      { "src/a.tsx": { cva: 1, lucideReact: 3, rawButton: 2 } },
      { "src/a.tsx": { cva: 1, lucideReact: 2, rawButton: 2 } },
    );
    assert.deepEqual(violations, ["src/a.tsx: lucideReact rose from 2 to 3"]);
  });

  it("passes a fall and reports that the ledger can shrink", () => {
    assert.deepEqual(compareToLedger({ "src/a.tsx": { rawButton: 4 } }, ledger), {
      violations: [],
      canShrink: true,
    });
    assert.deepEqual(compareToLedger(ledger, ledger), { violations: [], canShrink: false });
  });
});

describe("retiredImports", () => {
  it("names a retired component however a file reaches it, tests and stories included", () => {
    const source = `
      import { Dialog } from "@/components/ui/dialog";
      import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
      import { Button } from "@/components/ui/button";
      import { Menu } from "@base-ui/react/menu";
    `;
    assert.deepEqual(retiredImports(source, appFile("feature/view.stories.tsx")), ["dialog"]);
    assert.deepEqual(
      retiredImports(`import { Dialog } from "../ui/dialog";`, appFile("components/x/x.tsx")),
      ["dialog"],
    );
  });

  it("does not count the legacy component's own wiring", () => {
    const own = `import { Dialog as DialogPrimitive } from "@base-ui/react/dialog"; export * from "./dialog";`;
    assert.deepEqual(retiredImports(own, appFile("components/ui/dialog/dialog.tsx")), []);
    assert.deepEqual(retiredImports(own, appFile("components/ui/dialog/index.ts")), []);
  });
});

describe("compareRetired", () => {
  const retired = { dialog: ["src/old.tsx"] };

  it("fails a file that is not allowed to import a retired component", () => {
    assert.deepEqual(compareRetired({ "src/old.tsx": ["dialog"], "src/new.tsx": ["dialog"] }, retired), [
      "src/new.tsx: imports the retired dialog; use @loomarr/design-system's instead",
    ]);
  });

  it("fails an allowance that is no longer used, so the list only shrinks", () => {
    assert.deepEqual(compareRetired({}, retired), [
      "src/old.tsx: no longer imports the retired dialog; drop it from RETIRED in check-legacy-usage.mjs",
    ]);
  });
});

describe("shrinkLedger", () => {
  it("lowers counts and drops emptied files but never raises or adds", () => {
    const ledger = { "src/a.tsx": { rawButton: 10, cva: 1 }, "src/gone.tsx": { baseUi: 2 } };
    const current = { "src/a.tsx": { rawButton: 12 }, "src/new.tsx": { baseUi: 1 } };
    assert.deepEqual(shrinkLedger(current, ledger), { "src/a.tsx": { rawButton: 10 } });
  });
});
