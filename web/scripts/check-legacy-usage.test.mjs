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
  it("counts class tokens that reach a class name, and nothing else", () => {
    const source = `
      import { cn } from "@/lib/utils";
      const tone = "not a class";
      export function Row({ active, variant }: { active: boolean; variant: "ghost" | "solid" }) {
        return (
          <div className="flex gap-2" title="plain text">
            <span className={cn("px-2 py-1", active && "font-bold", variant === "ghost" ? "opacity-50" : "")} />
            <Button className={buttonVariants({ variant: "ghost", size: "sm" })} />
            <p className={\`text-sm \${active ? "underline" : "no-underline"}\`} />
          </div>
        );
      }
    `;
    // flex gap-2 (2) + px-2 py-1 font-bold opacity-50 (4) + text-sm underline no-underline (3).
    // Of those, text-sm is a raw type size and font-bold a raw type weight.
    assert.deepEqual(measureSource(source, appFile("feature/row.tsx")), {
      tailwindClasses: 9,
      rawTypeSize: 1,
      rawTypeWeight: 1,
    });
  });

  it("counts cva bases and variant values but not defaults or compound selectors", () => {
    const source = `
      import { cva, type VariantProps } from "class-variance-authority";
      export const badge = cva("inline-flex rounded", {
        variants: { tone: { neutral: "bg-muted text-muted-foreground", alert: "bg-destructive" } },
        compoundVariants: [{ tone: "alert", size: "sm", class: "ring-1" }],
        defaultVariants: { tone: "neutral" },
      });
    `;
    // inline-flex rounded (2) + bg-muted text-muted-foreground (2) + bg-destructive (1) + ring-1 (1).
    assert.deepEqual(measureSource(source, appFile("feature/badge.ts")), { tailwindClasses: 6, cva: 1 });
  });

  it("classifies every import form by the legacy module it names", () => {
    const source = `
      import { twMerge } from "tailwind-merge";
      import clsx from "clsx";
      import type { VariantProps } from "class-variance-authority";
      import { Dialog } from "@base-ui/react/dialog";
      import { Button } from "@/components/ui/button";
      export * from "@/components/ui";
      const lazy = import("@/components/ui/sheet");
      import { Card } from "../../components/ui/card";
      import { formatTime } from "@/lib/time";
    `;
    assert.deepEqual(measureSource(source, appFile("feature/nested/view.ts")), {
      tailwindMerge: 1,
      clsx: 1,
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

  it("counts raw type sizes and type weights separately, and each also as a Tailwind class", () => {
    const source = `
      export const Row = ({ on }: { on: boolean }) => (
        <div className="flex text-sm md:text-lg">
          <span className={cn("text-[13px] text-[0.8rem] text-muted-foreground", on && "font-semibold")} />
          <b className="font-mono font-bold tracking-wide leading-tight uppercase" />
          <i className="text-2xs dark:text-xl !text-base" />
        </div>
      );
    `;
    // Sizes: text-sm, md:text-lg, text-[13px], text-[0.8rem], text-2xs, dark:text-xl, !text-base.
    // Weights: font-semibold, font-bold, tracking-wide, leading-tight. font-mono and
    // text-muted-foreground are neither.
    assert.deepEqual(measureSource(source, appFile("feature/row.tsx")), {
      tailwindClasses: 15,
      rawTypeSize: 7,
      rawTypeWeight: 4,
    });
  });

  it("does not count a type role's size or weight that is not in a class name", () => {
    const source = `
      const size = "text-sm";
      export const Row = () => <Text textRole="cardLabel" tone="secondary" title="font-bold" />;
    `;
    assert.deepEqual(measureSource(source, appFile("feature/row.tsx")), {});
  });

  it("counts legacy component imports in CSS", () => {
    const css = `@import "tailwindcss";\n@import "@/components/ui/button/button.css";\n`;
    assert.deepEqual(measureSource(css, appFile("styles.css")), { legacyUi: 1 });
  });
});

describe("compareToLedger", () => {
  const ledger = { "src/a.tsx": { tailwindClasses: 10, cva: 1 } };

  it("fails a rise and a file the ledger does not list", () => {
    const { violations } = compareToLedger(
      { "src/a.tsx": { tailwindClasses: 11, cva: 1 }, "src/b.tsx": { baseUi: 1 } },
      ledger,
    );
    assert.deepEqual(violations, [
      "src/a.tsx: tailwindClasses rose from 10 to 11",
      "src/b.tsx: new file with legacy usage (baseUi 1)",
    ]);
  });

  it("fails a rise in the icon and raw-button measures", () => {
    const { violations } = compareToLedger(
      { "src/a.tsx": { tailwindClasses: 10, cva: 1, lucideReact: 3, rawButton: 2 } },
      { "src/a.tsx": { tailwindClasses: 10, cva: 1, lucideReact: 2, rawButton: 2 } },
    );
    assert.deepEqual(violations, ["src/a.tsx: lucideReact rose from 2 to 3"]);
  });

  it("fails a rise in the raw type size and weight measures", () => {
    const { violations } = compareToLedger(
      { "src/a.tsx": { tailwindClasses: 10, cva: 1, rawTypeSize: 3, rawTypeWeight: 2 } },
      { "src/a.tsx": { tailwindClasses: 10, cva: 1, rawTypeSize: 2, rawTypeWeight: 1 } },
    );
    assert.deepEqual(violations, [
      "src/a.tsx: rawTypeSize rose from 2 to 3",
      "src/a.tsx: rawTypeWeight rose from 1 to 2",
    ]);
  });

  it("passes a fall and reports that the ledger can shrink", () => {
    assert.deepEqual(compareToLedger({ "src/a.tsx": { tailwindClasses: 4 } }, ledger), {
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
    const ledger = { "src/a.tsx": { tailwindClasses: 10, cva: 1 }, "src/gone.tsx": { baseUi: 2 } };
    const current = { "src/a.tsx": { tailwindClasses: 12 }, "src/new.tsx": { baseUi: 1 } };
    assert.deepEqual(shrinkLedger(current, ledger), { "src/a.tsx": { tailwindClasses: 10 } });
  });
});
