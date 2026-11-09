import { CreateLocalUserInputBodyRole } from "@loomarr/api/models/createLocalUserInputBodyRole";
import { createLocalUserSchema } from "@loomarr/core/schemas";
import {
  Action,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@loomarr/design-system";
import { useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useAppForm } from "@/lib/app-form";
import type { CreateLocalDialogProps } from "./create-local-dialog.type";

const ROLE_OPTIONS = [
  { label: "Member", value: CreateLocalUserInputBodyRole.member },
  { label: "Admin", value: CreateLocalUserInputBodyRole.admin },
] as const;

const CreateLocalDialog = ({
  creating = false,
  defaultOpen = false,
  error,
  portalContainer,
  onCreate,
}: CreateLocalDialogProps) => {
  const [open, setOpen] = useState(defaultOpen);
  const form = useAppForm({
    defaultValues: {
      username: "",
      password: "",
      role: CreateLocalUserInputBodyRole.member as CreateLocalUserInputBodyRole,
    },
    validators: { onSubmit: createLocalUserSchema },
    onSubmit: async ({ value }) => {
      try {
        await onCreate(createLocalUserSchema.parse(value));
        form.reset();
        setOpen(false);
      } catch {
        // The connected caller exposes the RFC 7807 problem through `error`. Keep the
        // entered values available for correction while never persisting the password.
      }
    },
  });

  const changeOpen = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) form.reset();
  };

  // The dialog opens from its own state rather than a trigger part, so the opener can be an Action;
  // the Dialog returns focus to it on close all the same.
  return (
    <>
      <Action onPress={() => changeOpen(true)} variant="outline">
        Add local account
      </Action>
      <Dialog open={open} onOpenChange={changeOpen}>
        <DialogContent portalContainer={portalContainer}>
          <form
            className="flex flex-col gap-4"
            noValidate
            onSubmit={(event) => {
              event.preventDefault();
              void form.handleSubmit();
            }}
          >
            <DialogHeader>
              <DialogTitle>Add local account</DialogTitle>
              <DialogDescription>
                Create access for someone without an Emby or Jellyfin account. Loomarr stores only a
                non-reversible Argon2id verifier for this password.
              </DialogDescription>
            </DialogHeader>

            <form.AppField name="username">
              {(field) => <field.TextField className="gap-1.5" disabled={creating} label="Username" />}
            </form.AppField>
            <form.AppField name="password">
              {(field) => (
                <field.TextField className="gap-1.5" disabled={creating} label="Password" type="password" />
              )}
            </form.AppField>
            <form.AppField name="role">
              {(field) => <field.SelectField disabled={creating} label="Role" options={ROLE_OPTIONS} />}
            </form.AppField>

            {error != null && <ErrorState error={error} className="p-4" />}

            <DialogFooter>
              <Action disabled={creating} onPress={() => changeOpen(false)} variant="outline">
                Cancel
              </Action>
              <Action disabled={creating} submit>
                {creating ? "Creating…" : "Create account"}
              </Action>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
};

export { CreateLocalDialog };
