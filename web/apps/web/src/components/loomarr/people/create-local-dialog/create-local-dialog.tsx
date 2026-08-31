import type { CreateLocalUserInputBodyRole } from "@loomarr/api/models/createLocalUserInputBodyRole";
import { useId, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
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
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<CreateLocalUserInputBodyRole>("member");
  const [validationError, setValidationError] = useState("");

  const reset = () => {
    setUsername("");
    setPassword("");
    setRole("member");
    setValidationError("");
  };

  const changeOpen = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) reset();
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const trimmedUsername = username.trim();
    if (!trimmedUsername) {
      setValidationError("Pick a username.");
      return;
    }
    if (password.length < 8) {
      setValidationError("Use at least 8 characters.");
      return;
    }
    setValidationError("");
    try {
      await onCreate({ username: trimmedUsername, password, role });
      reset();
      setOpen(false);
    } catch {
      // The connected caller exposes the RFC 7807 problem through `error`. Keep the
      // entered values available for correction while never persisting the password.
    }
  };

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger render={<Button variant="outline" />}>Add local account</DialogTrigger>
      <DialogContent portalContainer={portalContainer}>
        <form className="flex flex-col gap-4" onSubmit={(event) => void submit(event)}>
          <DialogHeader>
            <DialogTitle>Add local account</DialogTitle>
            <DialogDescription>
              Create access for someone without an Emby or Jellyfin account. Loomarr stores only a
              non-reversible Argon2id verifier for this password.
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor={usernameId}>Username</Label>
            <Input
              id={usernameId}
              value={username}
              disabled={creating}
              onChange={(event) => setUsername(event.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={passwordId}>Password</Label>
            <Input
              id={passwordId}
              type="password"
              value={password}
              disabled={creating}
              onChange={(event) => setPassword(event.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={roleId}>Role</Label>
            <Select
              value={role}
              disabled={creating}
              onValueChange={(value) => setRole(value as CreateLocalUserInputBodyRole)}
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

          {validationError && (
            <p role="alert" className="text-destructive text-sm">
              {validationError}
            </p>
          )}

          {error != null && <ErrorState error={error} className="p-4" />}

          <DialogFooter>
            <DialogClose render={<Button type="button" variant="outline" disabled={creating} />}>
              Cancel
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
