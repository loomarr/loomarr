import { Action } from "@loomarr/design-system";
import { useStore } from "@tanstack/react-form";
import { useState } from "react";

import { useUiForm } from "../../../form-kit";
import { Line, Stack } from "../../library-poster";
import type { LibrarySettingsProps } from "../library-settings.type";

const AddFolder = ({ onAddFolder }: Pick<LibrarySettingsProps, "onAddFolder">) => {
  const [adding, setAdding] = useState(false);
  // The server's refusal (a path it cannot open) is not the form's own validation: it stays under the
  // field until the path is edited.
  const [error, setError] = useState<string>();
  const form = useUiForm({
    defaultValues: { path: "" },
    onSubmit: async ({ formApi, value }) => {
      const refused = await onAddFolder(value.path.trim());
      setError(refused);
      if (!refused) {
        formApi.reset();
        setAdding(false);
      }
    },
  });
  const path = useStore(form.store, (current) => current.values.path);
  const saving = useStore(form.store, (current) => current.isSubmitting);
  if (!adding) {
    return (
      <Line>
        <Action icon="plus" onPress={() => setAdding(true)} variant="outline">
          Add a folder
        </Action>
      </Line>
    );
  }
  const submit = () => void form.handleSubmit();
  return (
    <Line alignItems="flex-end" gap="control">
      <Stack flexBasis={240} flexGrow={1} flexShrink={1}>
        <form.AppField listeners={{ onChange: () => setError(undefined) }} name="path">
          {(field) => (
            <field.TextField
              error={error}
              label="Where is the folder?"
              onSubmitEditing={submit}
              placeholder="/media/…"
            />
          )}
        </form.AppField>
      </Stack>
      <Action disabled={path.trim() === "" || saving} onPress={submit} tone="primary">
        Add folder
      </Action>
      <Action
        onPress={() => {
          setAdding(false);
          form.reset();
          setError(undefined);
        }}
        variant="ghost"
      >
        Cancel
      </Action>
    </Line>
  );
};

export { AddFolder };
