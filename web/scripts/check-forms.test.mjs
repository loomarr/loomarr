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

  it("passes a form whose inputs write the form library's fields", () => {
    const converted = `
      import { useAppForm } from "@/lib/app-form";
      export const Name = () => {
        const form = useAppForm({ defaultValues: { name: "" } });
        return (
          <form.Field name="name">
            {(field) => <Field onChangeText={field.handleChange} value={field.state.value} />}
          </form.Field>
        );
      };
    `;
    assert.equal(isHandRolledForm(converted, "name.tsx"), false);
  });

  it("flags a useState draft beside a form, since importing the library excuses nothing else", () => {
    const beside = `import { useAppForm } from "@/lib/app-form";\n${handRolled}`;
    assert.equal(isHandRolledForm(beside, "name.tsx"), true);
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

  it("passes a setter that clears a value as the viewer types (an error the last submit raised)", () => {
    assert.equal(isHandRolledForm(editor("(next) => { setName(undefined); save(next); }")), false);
    assert.equal(isHandRolledForm(editor("(next) => { setName(null); save(next); }")), false);
  });

  it("still flags a setter that is handed what changed, or a computed boolean", () => {
    assert.equal(isHandRolledForm(editor("(next) => { setName(next); }")), true);
    assert.equal(isHandRolledForm(editor("() => setRequested(!requested)")), true);
    assert.equal(isHandRolledForm(editor("(next) => { setRequested(true); setName(next); }")), true);
    assert.equal(isHandRolledForm(editor("(next) => setName('fixed')")), true);
    assert.equal(isHandRolledForm(editor("setRequested")), true);
  });
});

describe("isHandRolledForm, a draft written through a wrapper", () => {
  const note = (declaration, attribute = "onChangeText={changeNote}") => `
    import { useState } from "react";
    export const Pull = ({ onEdited }) => {
      const [note, setNote] = useState("");
      const [open, setOpen] = useState(false);
      ${declaration}
      return <Field ${attribute} value={note} />;
    };
  `;

  it("flags an input handled by a function that writes the setter", () => {
    assert.equal(
      isHandRolledForm(
        note("const changeNote = (value) => { setNote(value); onEdited(value.trim() !== ''); };"),
      ),
      true,
    );
    assert.equal(
      isHandRolledForm(note("function changeNote(value) { setNote(value); }", "onChangeText={changeNote}")),
      true,
    );
    assert.equal(isHandRolledForm(note("const changeNote = function (value) { setNote(value); };")), true);
  });

  it("follows a wrapper that calls another wrapper", () => {
    assert.equal(
      isHandRolledForm(
        note("const store = (value) => setNote(value); const changeNote = (value) => { store(value); };"),
      ),
      true,
    );
  });

  it("passes a wrapper that writes no draft, or only raises a flag", () => {
    assert.equal(isHandRolledForm(note("const changeNote = (value) => { onEdited(value); };")), false);
    assert.equal(isHandRolledForm(note("const changeNote = () => { setOpen(true); };")), false);
  });

  it("does not follow a wrapper that no input handler names", () => {
    assert.equal(
      isHandRolledForm(
        note("const save = (value) => { setNote(value); };", "onPress={save} onChangeText={onEdited}"),
      ),
      false,
    );
  });

  it("follows a DOM input's onChange, but not a component's, which reports a structured value", () => {
    const declaration = "const changeNote = (value) => { setNote(value); };";
    assert.equal(
      isHandRolledForm(note(declaration, "onChange={changeNote}").replace("<Field", "<input")),
      true,
    );
    assert.equal(isHandRolledForm(note(declaration, "onChange={changeNote}")), false);
  });

  it("passes a wrapper that resets state beside what an input handed it", () => {
    assert.equal(
      isHandRolledForm(note("const changeNote = (value) => { onEdited(value); setOpen([]); };")),
      false,
    );
  });

  it("terminates on a wrapper that calls itself", () => {
    assert.equal(isHandRolledForm(note("const changeNote = (value) => { changeNote(value); };")), false);
  });
});

describe("isHandRolledForm, a text setter passed as a prop", () => {
  const panel = (attribute, initial = '("")') => `
    import { useState } from "react";
    export const Page = () => {
      const [dsn, setDsn] = useState${initial};
      return <Form ${attribute} dsn={dsn} />;
    };
  `;

  it("flags a text state's setter handed to a change prop", () => {
    assert.equal(isHandRolledForm(panel("onDsnChange={setDsn}")), true);
    assert.equal(isHandRolledForm(panel("onDsnChangeText={setDsn}")), true);
    assert.equal(isHandRolledForm(panel("onDsnChange={setDsn}", "<string>(initial)")), true);
    assert.equal(isHandRolledForm(panel("onDsnChange={setDsn}", "(`x`)")), true);
  });

  it("passes a setter of state that is not text, a flag, or a view choice", () => {
    assert.equal(isHandRolledForm(panel("onStepChange={setDsn}", "(null)")), false);
    assert.equal(isHandRolledForm(panel("onOpenChange={setDsn}")), false);
    assert.equal(isHandRolledForm(panel("onValueChange={setDsn}")), false);
    assert.equal(isHandRolledForm(panel("onCheckedChange={setDsn}")), false);
    assert.equal(isHandRolledForm(panel("onPress={setDsn}")), false);
  });

  it("passes a search box's setter", () => {
    assert.equal(
      isHandRolledForm(`
        import { useState } from "react";
        export const Page = () => {
          const [query, setQuery] = useState("");
          return <List onQueryChange={setQuery} query={query} />;
        };
      `),
      false,
    );
  });
});

describe("isHandRolledForm, a filter's name does not excuse a form", () => {
  const fields = (wrapper, tail = "") => `
    import { useState } from "react";
    export const Server = () => {
      const [statusName, setStatusName] = useState("");
      const [filterHost, setFilterHost] = useState("");
      return (
        ${wrapper.replace(
          "$",
          `<>
            <Field onChangeText={setStatusName} value={statusName} />
            <Field onChangeText={setFilterHost} value={filterHost} />
            ${tail}
          </>`,
        )}
      );
    };
  `;

  it("flags two text inputs in a <form>, whatever their setters are called", () => {
    assert.equal(isHandRolledForm(fields("<form>$</form>")), true);
    assert.equal(isHandRolledForm(fields("<form onSubmit={save}>$</form>")), true);
  });

  it("flags text inputs beside a submit", () => {
    assert.equal(isHandRolledForm(fields("$", '<button type="submit">Save</button>')), true);
    assert.equal(isHandRolledForm(fields("<Stack onSubmit={save}>$</Stack>")), true);
  });

  it("still passes the same inputs as a lone search and filter", () => {
    assert.equal(isHandRolledForm(fields("$")), false);
    assert.equal(isHandRolledForm(fields("<Stack>$</Stack>")), false);
  });

  it("flags a filter-named text state handed whole to a change prop inside a form", () => {
    const source = `
      import { useState } from "react";
      export const Page = () => {
        const [filterHost, setFilterHost] = useState("");
        return (
          <form>
            <Hosts onHostChange={setFilterHost} host={filterHost} />
          </form>
        );
      };
    `;
    assert.equal(isHandRolledForm(source), true);
    assert.equal(isHandRolledForm(source.replace("<form>", "<div>").replace("</form>", "</div>")), false);
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

  it("is sanctioned from the UI package's entry point, where the app imports it", () => {
    const fromPackage = sanctioned.replace("../use-committed-draft", "@loomarr/ui");
    assert.equal(isHandRolledForm(fromPackage), false);
    assert.equal(usesCommittedDraft(fromPackage), true);
    // A hook of that name from anywhere else is not the buffer.
    assert.equal(isHandRolledForm(sanctioned.replace("../use-committed-draft", "@elsewhere/hooks")), true);
  });

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

  it("is one input's draft: two in one component are a form kept in renamed state", () => {
    const twoDrafts = `
      import { useCommittedDraft } from "@loomarr/ui";
      export const Contact = ({ person }) => {
        const { draft: name, editDraft: setName } = useCommittedDraft(person.name);
        const { draft: email, editDraft: setEmail } = useCommittedDraft(person.email);
        return (
          <>
            <Field onChangeText={setName} value={name} />
            <Field onChangeText={setEmail} value={email} />
          </>
        );
      };
    `;
    assert.equal(isHandRolledForm(twoDrafts), true);
    // One draft each in two components of the same file is two single inputs.
    const twoBoxes = `
      import { useCommittedDraft } from "@loomarr/ui";
      const NameBox = ({ person }) => {
        const { draft, editDraft } = useCommittedDraft(person.name);
        return <Field onChangeText={editDraft} value={draft} />;
      };
      export const EmailBox = ({ person }) => {
        const { draft, editDraft } = useCommittedDraft(person.email);
        return <Field onChangeText={editDraft} value={draft} />;
      };
    `;
    assert.equal(isHandRolledForm(twoBoxes), false);
  });

  it("counts a same-file wrapper hook as the hook it calls, so a renamed pair is still a form", () => {
    const direct = `
      import { useCommittedDraft } from "@loomarr/ui";
      export const Server = () => {
        const name = useCommittedDraft("");
        const host = useCommittedDraft("");
        return (
          <form>
            <Field onChangeText={name.editDraft} value={name.draft} />
            <Field onChangeText={host.editDraft} value={host.draft} />
          </form>
        );
      };
    `;
    assert.equal(isHandRolledForm(direct), true);
    const wrapped = `
      import { useCommittedDraft } from "@loomarr/ui";
      const useName = () => useCommittedDraft("");
      const useHost = () => useCommittedDraft("");
      export const Server = () => {
        const name = useName();
        const host = useHost();
        return (
          <form>
            <Field onChangeText={name.editDraft} value={name.draft} />
            <Field onChangeText={host.editDraft} value={host.draft} />
          </form>
        );
      };
    `;
    assert.equal(isHandRolledForm(wrapped), true);
    // A wrapper of a wrapper, as a function declaration.
    assert.equal(
      isHandRolledForm(
        wrapped.replace(
          'const useHost = () => useCommittedDraft("");',
          "function useHost() { return useName(); }",
        ),
      ),
      true,
    );
    // One wrapper used twice.
    assert.equal(isHandRolledForm(wrapped.replace("useHost();", "useName();")), true);
    // One wrapped draft in one component is still a single input.
    const single = `
      import { useCommittedDraft } from "@loomarr/ui";
      const useName = () => useCommittedDraft("");
      export const Box = () => {
        const name = useName();
        return <Field onChangeText={name.editDraft} value={name.draft} />;
      };
    `;
    assert.equal(isHandRolledForm(single), false);
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
