import type { CreateLocalUserInputBodyRole } from "@loomarr/api/models/createLocalUserInputBodyRole";
import { createLocalUserSchema } from "@loomarr/core/schemas";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@loomarr/design-system";
import { useForm } from "@tanstack/react-form";
import { useId, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { FieldError } from "@/components/loomarr/feedback/field-error";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { CreateLocalDialogProps } from "./create-local-dialog.type";

const CreateLocalDialog = ({
  creating = false,
  defaultOpen = false,
  error,
  portalContainer,
  onCreate,
}: CreateLocalDialogProps) => {
  const usernameId = useId();
  const passwordId = useId();
  const roleId = useId();
  const [open, setOpen] = useState(defaultOpen);
  const form = useForm({
    defaultValues: { username: "", password: "", role: "member" as CreateLocalUserInputBodyRole },
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

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger>
        <Button variant="outline">Add local account</Button>
      </DialogTrigger>
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

          <form.Field name="username">
            {(field) => (
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={usernameId}>Username</Label>
                <Input
                  id={usernameId}
                  name={field.name}
                  value={field.state.value}
                  disabled={creating}
                  aria-invalid={field.state.meta.errors.length > 0 ? "true" : undefined}
                  onChange={(event) => field.handleChange(event.target.value)}
                />
                <FieldError errors={field.state.meta.errors} />
              </div>
            )}
          </form.Field>
          <form.Field name="password">
            {(field) => (
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={passwordId}>Password</Label>
                <Input
                  id={passwordId}
                  name={field.name}
                  type="password"
                  value={field.state.value}
                  disabled={creating}
                  aria-invalid={field.state.meta.errors.length > 0 ? "true" : undefined}
                  onChange={(event) => field.handleChange(event.target.value)}
                />
                <FieldError errors={field.state.meta.errors} />
              </div>
            )}
          </form.Field>
          <form.Field name="role">
            {(field) => (
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={roleId}>Role</Label>
                <Select
                  value={field.state.value}
                  disabled={creating}
                  onValueChange={(value) => field.handleChange(value as CreateLocalUserInputBodyRole)}
                >
                  <SelectTrigger id={roleId}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="member">Member</SelectItem>
                    <SelectItem value="admin">Admin</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            )}
          </form.Field>

          {error != null && <ErrorState error={error} className="p-4" />}

          <DialogFooter>
            <DialogClose>
              <Button type="button" variant="outline" disabled={creating}>
                Cancel
              </Button>
            </DialogClose>
            <Button type="submit" disabled={creating}>
              {creating ? "Creating…" : "Create account"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
};

export { CreateLocalDialog };
