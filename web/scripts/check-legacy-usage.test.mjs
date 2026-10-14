import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { compareToLedger, measureSource, shrinkLedger } from "./check-legacy-usage.mjs";

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
    assert.deepEqual(measureSource(source, appFile("feature/row.tsx")), { tailwindClasses: 9 });
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
      import { tokens } from "@loomarr/tokens";
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
      legacyTokens: 1,
    });
  });

  it("does not count a copied component's own wiring as a consumer", () => {
    const own = `export * from "./button"; export type { ButtonProps } from "./button.type";`;
    assert.deepEqual(measureSource(own, appFile("components/ui/button/index.ts")), {});
    const other = `import { Button } from "../button";`;
    assert.deepEqual(measureSource(other, appFile("components/ui/dialog/dialog.tsx")), { legacyUi: 1 });
  });

  it("counts legacy token imports in CSS", () => {
    const css = `@import "tailwindcss";\n@import "@loomarr/tokens/theme.css";\n`;
    assert.deepEqual(measureSource(css, appFile("styles.css")), { legacyTokens: 1 });
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

  it("passes a fall and reports that the ledger can shrink", () => {
    assert.deepEqual(compareToLedger({ "src/a.tsx": { tailwindClasses: 4 } }, ledger), {
      violations: [],
      canShrink: true,
    });
    assert.deepEqual(compareToLedger(ledger, ledger), { violations: [], canShrink: false });
  });
});

describe("shrinkLedger", () => {
  it("lowers counts and drops emptied files but never raises or adds", () => {
    const ledger = { "src/a.tsx": { tailwindClasses: 10, cva: 1 }, "src/gone.tsx": { baseUi: 2 } };
    const current = { "src/a.tsx": { tailwindClasses: 12 }, "src/new.tsx": { baseUi: 1 } };
    assert.deepEqual(shrinkLedger(current, ledger), { "src/a.tsx": { tailwindClasses: 10 } });
  });
});
