import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  committedDraftUsers,
  compareToLedger,
  isHandRolledForm,
  settingsDraftUsers,
  shrinkLedger,
  usesCommittedDraft,
  usesSettingsDraft,
} from "./check-forms.mjs";

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

describe("isHandRolledForm, what is not a form", () => {
  it("passes a search box, a list filter and a select that picks a view", () => {
    const search = handRolled.replace("setName", "setQuery").replace("setName", "setQuery");
    assert.equal(isHandRolledForm(search), false);
    const select = `
      import { useState } from "react";
      export const View = () => {
        const [view, setView] = useState("day");
        return <Select onValueChange={setView} value={view} />;
      };
    `;
    assert.equal(isHandRolledForm(select), false);
  });
});

describe("isHandRolledForm, a flag a handler raises", () => {
  const editor = (handler) => `
    import { useState } from "react";
    export const Dates = () => {
      const [requested, setRequested] = useState(false);
      const [name, setName] = useState("");
      return <DateAxisEditor onChange={${handler}} requested={requested} name={name} />;
    };
  `;

  it("passes a setter called only with a boolean literal", () => {
    assert.equal(isHandRolledForm(editor("(next) => { setRequested(true); save(next); }")), false);
  });

  it("still flags a setter that is handed what changed, or a computed boolean", () => {
    assert.equal(isHandRolledForm(editor("(next) => { setName(next); }")), true);
    assert.equal(isHandRolledForm(editor("() => setRequested(!requested)")), true);
    assert.equal(isHandRolledForm(editor("(next) => { setRequested(true); setName(next); }")), true);
    assert.equal(isHandRolledForm(editor("(next) => setName('fixed')")), true);
    assert.equal(isHandRolledForm(editor("setRequested")), true);
  });
});

describe("the committed draft", () => {
  const sanctioned = `
    import { useCommittedDraft } from "../use-committed-draft";
    export const Box = ({ initial, onCommit }) => {
      const { draft, editDraft } = useCommittedDraft(initial);
      return <Field onBlur={() => onCommit(draft)} onChangeText={editDraft} value={draft} />;
    };
  `;

  it("is the sanctioned draft of a single input, and is named, not merely unnoticed", () => {
    assert.equal(isHandRolledForm(sanctioned), false);
    assert.equal(usesCommittedDraft(sanctioned), true);
    assert.equal(usesSettingsDraft(sanctioned), false);
    // Renamed locally, still the hook's setter.
    const renamed = sanctioned
      .replace("editDraft }", "editDraft: setText }")
      .replace("={editDraft}", "={setText}");
    assert.equal(isHandRolledForm(renamed), false);
    assert.equal(usesCommittedDraft(renamed), true);
  });

  it("flags a setter from a hook that is not the sanctioned one", () => {
    const lookalike = sanctioned.replace('"../use-committed-draft"', '"./my-own-draft"');
    assert.equal(isHandRolledForm(lookalike), true);
    assert.equal(usesCommittedDraft(lookalike), false);
    const renamedHook = sanctioned
      .replace(/useCommittedDraft/g, "useDraft")
      .replace("use-committed-draft", "draft");
    assert.equal(isHandRolledForm(renamedHook), true);
    // The settings hooks do not sanction a committed draft's module, nor the other way round.
    const crossed = sanctioned.replace("../use-committed-draft", "@/settings/settings-draft");
    assert.equal(isHandRolledForm(crossed), true);
  });

  it("flags the [value, setValue] pair a lookalike of useState returns", () => {
    const pair = `
      import { useLocalDraft } from "./local-draft";
      export const Box = () => {
        const [name, setName] = useLocalDraft("");
        return <Field onChangeText={setName} value={name} />;
      };
    `;
    assert.equal(isHandRolledForm(pair), true);
    assert.equal(isHandRolledForm(pair.replace(/setName/g, "setQuery")), false);
  });

  it("does not excuse a hand-rolled input in a file that also uses the hook", () => {
    const mixed = `${sanctioned.replace("export const Box", "const Box")}
      export const Name = () => {
        const [name, setName] = useState("");
        return <Field onChangeText={setName} value={name} />;
      };
    `;
    assert.equal(isHandRolledForm(mixed), true);
  });

  it("is how the single-input field primitives and the path-map row keep their draft", () => {
    const users = committedDraftUsers();
    for (const file of [
      "packages/ui/src/commit-field/commit-field.tsx",
      "packages/ui/src/number-field/number-field.tsx",
      "packages/ui/src/year-range-list-field/year-range-list-field.tsx",
      "packages/ui/src/library/library-settings/library-settings.tsx",
    ]) {
      assert.ok(users.includes(file), `${file} uses the committed draft`);
    }
  });
});

describe("the settings draft", () => {
  const sanctioned = `
    import { SettingsFields } from "@/components/loomarr/settings/settings-fields";
    import { useSettingsDraft } from "@/settings/settings-draft";
    export const Step = () => {
      const { edits, setEdit } = useSettingsDraft();
      return <SettingsFields entries={[]} onChange={setEdit} values={edits} />;
    };
  `;

  it("is the sanctioned way to buffer edits, and is named, not merely unnoticed", () => {
    assert.equal(isHandRolledForm(sanctioned), false);
    assert.equal(usesSettingsDraft(sanctioned), true);
    // The layout's buffer is the same hook underneath, and is sanctioned under its own name.
    const layout = sanctioned
      .replace("useSettingsDraft", "useSettingsEdits")
      .replace("settings-draft", "settings-edits")
      .replace("useSettingsDraft()", "useSettingsEdits()");
    assert.equal(isHandRolledForm(layout), false);
    assert.equal(usesSettingsDraft(layout), true);
    // Renamed locally, still the hook's setter.
    assert.equal(
      isHandRolledForm(
        sanctioned
          .replace("setEdit }", "setEdit: setLocationEdit }")
          .replace("={setEdit}", "={setLocationEdit}"),
      ),
      false,
    );
  });

  it("flags the same buffer kept in useState", () => {
    const hand = `
      import { useState } from "react";
      export const Step = () => {
        const [edits, setEdits] = useState({});
        return <SettingsFields onChange={(key, value) => setEdits({ ...edits, [key]: value })} values={edits} />;
      };
    `;
    assert.equal(isHandRolledForm(hand), true);
    assert.equal(usesSettingsDraft(hand), false);
  });

  it("flags a draft setter from a hook that is not the sanctioned one", () => {
    const lookalike = `
      import { useSettingsDraft } from "./my-own-draft";
      export const Step = () => {
        const { edits, setEdit } = useSettingsDraft();
        return <SettingsFields onChange={setEdit} values={edits} />;
      };
    `;
    assert.equal(isHandRolledForm(lookalike), true);
    assert.equal(usesSettingsDraft(lookalike), false);
    const other = lookalike.replace('"./my-own-draft"', '"@/settings/settings-draft"');
    assert.equal(isHandRolledForm(other), false);
    const another = other
      .replace(/useSettingsDraft/g, "useDraft")
      .replace("@/settings/settings-draft", "./draft");
    assert.equal(isHandRolledForm(another), true);
  });

  it("does not excuse a hand-rolled input in a file that also uses the hook", () => {
    const mixed = `${sanctioned.replace("export const Step", "const Step")}
      export const Name = () => {
        const [name, setName] = useState("");
        return <Field onChangeText={setName} value={name} />;
      };
    `;
    assert.equal(isHandRolledForm(mixed), true);
  });

  it("is how Settings and every wizard step buffer their edits", () => {
    const users = settingsDraftUsers();
    for (const file of [
      "apps/web/src/settings/settings-edits/settings-edits.tsx",
      "apps/web/src/settings/settings-page/settings-page.tsx",
      "apps/web/src/routes/wizard.tsx",
      "apps/web/src/wizard/checklist-step/checklist-step.tsx",
      "apps/web/src/wizard/library-step/library-step.tsx",
      "apps/web/src/wizard/playout-step/playout-step.tsx",
      "apps/web/src/wizard/wizard-ai-block/wizard-ai-block.tsx",
    ]) {
      assert.ok(users.includes(file), `${file} uses the settings draft`);
    }
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
