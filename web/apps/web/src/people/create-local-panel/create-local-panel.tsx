import * as usersApi from "@loomarr/api/endpoints/users";
import type { CreateLocalUserInputBodyRole } from "@loomarr/api/models/createLocalUserInputBodyRole";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { useId, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { CreateLocalPanelProps } from "./create-local-panel.type";

// CreateLocalPanel — the second way into §11's allowlist, alongside import.
//
// Before this, `POST /v1/setup/bootstrap` was the ONLY path that made a local user and
// it succeeds exactly once, so an install had exactly one account with a Loomarr-stored
// password, forever. This is for the person who has no media-server account at all.
//
// It does NOT weaken the allowlist: like import, it is an explicit admin action that
// adds a row. Signing in still provisions nobody.
const CreateLocalPanel = ({ onCreated, initiallyOpen = false, className }: CreateLocalPanelProps) => {
  const nameId = useId();
  const pwId = useId();
  const roleId = useId();

  const [open, setOpen] = useState(initiallyOpen);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  // Typed from the generated enum, not `string`: if the role set ever changes, the
  // compiler says so here rather than the server rejecting it at runtime (§14 — FE
  // request types come from orval, never hand-written).
  const [role, setRole] = useState<CreateLocalUserInputBodyRole>("member");
  const [error, setError] = useState("");

  const create = usersApi.useCreateLocalUser({
    mutation: {
      onSuccess: (response) => {
        toast.success(`Created ${unwrap(response, (body) => body.name) ?? username}`);
        setUsername("");
        setPassword("");
        setRole("member");
        setError("");
        setOpen(false);
        onCreated?.();
      },
      // The server's problem detail carries the useful part — "Someone already signs in
      // with that username" reads better than anything generic we'd substitute.
      onError: (e) => setError(toProblem(e).detail ?? "Couldn't create that account."),
    },
  });

  const submit = () => {
    if (!username.trim()) return setError("Pick a username.");
    if (password.length < 8) return setError("Use at least 8 characters.");
    setError("");
    create.mutate({ data: { username: username.trim(), password, role } });
  };

  return (
    <Card className={cn("flex flex-col gap-3 p-4", className)}>
      <div>
        <h2 className="font-medium">Add someone without a media-server account</h2>
        <p className="mt-1 text-muted-foreground text-sm">
          For someone with no media-server account. Loomarr stores this password itself. It is the only kind
          of account it can reset.
        </p>
      </div>

      {!open ? (
        <Button variant="outline" className="w-fit" onClick={() => setOpen(true)}>
          Create local account
        </Button>
      ) : (
        <div className="flex flex-col gap-3">
          <div className="grid gap-3 sm:grid-cols-3">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor={nameId}>Username</Label>
              <Input id={nameId} value={username} onChange={(e) => setUsername(e.target.value)} />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor={pwId}>Password</Label>
              <Input
                id={pwId}
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor={roleId}>Role</Label>
              {/* Defaults to member, matching the server: minting an admin is a
                  deliberate choice, never what happens when you don't look (§11). */}
              <Select value={role} onValueChange={(v) => setRole(v as CreateLocalUserInputBodyRole)}>
                <SelectTrigger id={roleId}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="member">member</SelectItem>
                  <SelectItem value="admin">admin</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          {error && <p className="text-onair-300 text-sm">{error}</p>}
          <div className="flex gap-2">
            <Button onClick={submit} disabled={create.isPending}>
              Create account
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                setOpen(false);
                setError("");
              }}
              disabled={create.isPending}
            >
              Cancel
            </Button>
          </div>
        </div>
      )}
    </Card>
  );
};

export { CreateLocalPanel };
