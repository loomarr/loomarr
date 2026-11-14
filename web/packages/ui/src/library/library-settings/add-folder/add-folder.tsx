import { Action } from "@loomarr/design-system";
import { useStore } from "@tanstack/react-form";
import { useState } from "react";

import { useUiForm } from "../../../form-kit";
import { Line, Stack } from "../../library-poster";
import { FolderBrowser } from "../folder-browser";
import type { LibrarySettingsProps } from "../library-settings.type";

const AddFolder = ({
  onAddFolder,
  folderBrowser,
}: Pick<LibrarySettingsProps, "onAddFolder" | "folderBrowser">) => {
  const [adding, setAdding] = useState(false);
  // With a browser, a folder is picked by opening folders; "Type a path instead" turns to the field.
  const [typing, setTyping] = useState(false);
  // The server's refusal (a path it cannot open) is not the form's own validation: it stays under the
  // field until the path is edited.
  const [error, setError] = useState<string>();
  const [picking, setPicking] = useState(false);
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
  const close = () => {
    setAdding(false);
    setTyping(false);
    form.reset();
    setError(undefined);
  };
  if (!adding) {
    return (
      <Line>
        <Action
          icon="plus"
          onPress={() => {
            setAdding(true);
            folderBrowser?.onOpen("");
          }}
          variant="outline"
        >
          Add a folder
        </Action>
      </Line>
    );
  }
  if (folderBrowser && !typing) {
    const use = async (path: string, replace: boolean) => {
      setPicking(true);
      const refused = await onAddFolder(path, replace ? { replace } : undefined);
      setPicking(false);
      setError(refused);
      if (!refused) close();
    };
    return (
      <FolderBrowser
        browser={{
          ...folderBrowser,
          onOpen: (path) => {
            setError(undefined);
            folderBrowser.onOpen(path);
          },
        }}
        onCancel={close}
        onTypePath={() => {
          setError(undefined);
          setTyping(true);
        }}
        onUse={(path, replace) => void use(path, replace)}
        refusal={error}
        saving={picking}
      />
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
      <Action onPress={close} variant="ghost">
        Cancel
      </Action>
    </Line>
  );
};

export { AddFolder };
