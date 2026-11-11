import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { compareToLedger, isHandRolledForm, shrinkLedger } from "./check-forms.mjs";

const handRolled = `
  import { useState } from "react";
  export const Name = () => {
    const [name, setName] = useState("");
    return <Field onChangeText={(text) => { setName(text); }} value={name} />;
  };
`;

describe("isHandRolledForm", () => {
  it("flags an input whose draft lives in useState", () => {
    assert.equal(isHandRolledForm(handRolled, "name.tsx"), true);
    assert.equal(
      isHandRolledForm(
        handRolled.replace("onChangeText={(text) => { setName(text); }}", "onChangeText={setName}"),
        "name.tsx",
      ),
      true,
    );
  });

  it("passes a form that imports the form library", () => {
    const converted = `import { useAppForm } from "@/lib/app-form";\n${handRolled}`;
    assert.equal(isHandRolledForm(converted, "name.tsx"), false);
  });

  it("ignores state no input writes", () => {
    const source = `
      import { useState } from "react";
      export const Panel = () => {
        const [open, setOpen] = useState(false);
        return <Action onPress={() => setOpen(true)} />;
      };
    `;
    assert.equal(isHandRolledForm(source, "panel.tsx"), false);
  });
});

describe("compareToLedger", () => {
  it("fails a form the ledger does not list and notes one that could go", () => {
    assert.deepEqual(compareToLedger(["a.tsx", "b.tsx"], ["a.tsx", "c.tsx"]), {
      canShrink: true,
      violations: ["b.tsx"],
    });
  });

  it("passes when forms were converted", () => {
    assert.deepEqual(compareToLedger(["a.tsx"], ["a.tsx", "b.tsx"]), {
      canShrink: true,
      violations: [],
    });
  });
});

describe("shrinkLedger", () => {
  it("drops converted forms and never adds one", () => {
    assert.deepEqual(shrinkLedger(["a.tsx", "new.tsx"], ["a.tsx", "b.tsx"]), ["a.tsx"]);
  });
});
