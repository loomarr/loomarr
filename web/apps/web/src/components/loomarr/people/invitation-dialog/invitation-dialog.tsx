import type { CreateInvitationInputBodyKind } from "@loomarr/api/models/createInvitationInputBodyKind";
import type { CreateInvitationInputBodyRole } from "@loomarr/api/models/createInvitationInputBodyRole";
import type { InvitationBody } from "@loomarr/api/models/invitationBody";
import type { IssueInvitationGrantOutputBody } from "@loomarr/api/models/issueInvitationGrantOutputBody";
import { QrCode } from "@loomarr/design-system/qr-code-web";
import { Copy, RefreshCw, Trash2, UserPlus } from "lucide-react";
import { useId, useRef, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import type { InvitationDialogProps } from "./invitation-dialog.type";

const invitationName = (value: InvitationBody) =>
  value.displayName || value.username || value.libraryUserId || "this person";

const expiryLabel = (expiresAt: number) =>
  new Intl.DateTimeFormat("en-US", {
    day: "numeric",
    month: "long",
    timeZone: "UTC",
    year: "numeric",
  }).format(expiresAt);

const InvitationDialog = ({
  candidates,
  defaultOpen = false,
  existing,
  libraryAvailable,
  open: controlledOpen,
  portalContainer,
  onCreate,
  onIssueGrant,
  onOpenChange,
  onRevoke,
  onSendEmail,
}: InvitationDialogProps) => {
  const usernameId = useId();
  const contactId = useId();
  const accountId = useId();
  const roleId = useId();
  const emailId = useId();
  const operation = useRef(0);
  const [internalOpen, setInternalOpen] = useState(defaultOpen);
  const [kind, setKind] = useState<CreateInvitationInputBodyKind>("local");
  const [username, setUsername] = useState("");
  const [libraryUserId, setLibraryUserId] = useState("");
  const [contactEmail, setContactEmail] = useState("");
  const [role, setRole] = useState<CreateInvitationInputBodyRole>("member");
  const [sendEmail, setSendEmail] = useState(false);
  const [invitation, setInvitation] = useState<InvitationBody | undefined>(existing);
  const [grant, setGrant] = useState<IssueInvitationGrantOutputBody>();
  const [copied, setCopied] = useState(false);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<unknown>();
  const [emailProblem, setEmailProblem] = useState(false);
  const open = controlledOpen ?? internalOpen;

  const reset = () => {
    operation.current += 1;
    setKind("local");
    setUsername("");
    setLibraryUserId("");
    setContactEmail("");
    setRole("member");
    setSendEmail(false);
    setInvitation(existing);
    setGrant(undefined);
    setCopied(false);
    setBusy(false);
    setProblem(undefined);
    setEmailProblem(false);
  };

  const changeOpen = (next: boolean) => {
    if (controlledOpen === undefined) setInternalOpen(next);
    onOpenChange?.(next);
    if (!next) reset();
    else if (existing) setInvitation(existing);
  };

  const issue = async (value: InvitationBody) => {
    const current = operation.current;
    setBusy(true);
    setProblem(undefined);
    setCopied(false);
    setGrant(undefined);
    try {
      const issued = await onIssueGrant(value.id, "qr");
      if (operation.current === current) setGrant(issued);
      return issued;
    } catch (error) {
      if (operation.current === current) setProblem(error);
      return undefined;
    } finally {
      if (operation.current === current) setBusy(false);
    }
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const current = operation.current;
    setProblem(undefined);
    setEmailProblem(false);
    if (kind === "local" && !username.trim()) {
      setProblem(new Error("Choose a username to reserve."));
      return;
    }
    if (kind === "library" && !libraryUserId) {
      setProblem(new Error("Choose an Emby or Jellyfin account."));
      return;
    }
    setBusy(true);
    try {
      const created = await onCreate({
        kind,
        ...(kind === "local" ? { username: username.trim() } : { libraryUserId }),
        contactEmail: contactEmail.trim(),
        role,
      });
      if (operation.current !== current) return;
      setInvitation(created);
      const issued = await onIssueGrant(created.id, "qr");
      if (operation.current !== current) return;
      setGrant(issued);
      if (sendEmail && contactEmail.trim() && onSendEmail) {
        try {
          await onSendEmail(created.id);
        } catch {
          if (operation.current === current) setEmailProblem(true);
        }
      }
    } catch (error) {
      if (operation.current === current) setProblem(error);
    } finally {
      if (operation.current === current) setBusy(false);
    }
  };

  const revoke = async () => {
    if (!invitation) return;
    const current = operation.current;
    setBusy(true);
    setProblem(undefined);
    setGrant(undefined);
    try {
      await onRevoke(invitation.id);
      if (operation.current === current) setInvitation({ ...invitation, status: "revoked" });
    } catch (error) {
      if (operation.current === current) setProblem(error);
    } finally {
      if (operation.current === current) setBusy(false);
    }
  };

  const selectedCandidate = candidates?.find((candidate) => candidate.id === libraryUserId);
  const terminal = invitation?.status !== undefined && invitation.status !== "pending";
  const title = existing ? `Share ${invitationName(existing)}’s invitation` : "Invite person";

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      {controlledOpen === undefined && (
        <DialogTrigger render={<Button />}>
          <UserPlus aria-hidden /> Invite person
        </DialogTrigger>
      )}
      <DialogContent
        portalContainer={portalContainer}
        className="max-h-[calc(100dvh-2rem)] max-w-2xl overflow-auto"
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            Reserve access, then share a short-lived link. Creating an invitation does not activate an
            account.
          </DialogDescription>
        </DialogHeader>

        {!invitation ? (
          <form className="flex flex-col gap-4" onSubmit={(event) => void submit(event)}>
            <fieldset className="grid gap-2 sm:grid-cols-2">
              <legend className="mb-2 font-medium text-sm">Credential path</legend>
              <Label className="flex min-h-11 cursor-pointer items-center gap-3 rounded-lg border border-border p-3">
                <input
                  type="radio"
                  name="invitation-kind"
                  value="local"
                  checked={kind === "local"}
                  disabled={busy}
                  onChange={() => setKind("local")}
                />
                Local Loomarr account
              </Label>
              <Label className="flex min-h-11 cursor-pointer items-center gap-3 rounded-lg border border-border p-3">
                <input
                  type="radio"
                  name="invitation-kind"
                  value="library"
                  checked={kind === "library"}
                  disabled={busy || !libraryAvailable}
                  onChange={() => setKind("library")}
                />
                Emby or Jellyfin account
              </Label>
            </fieldset>

            {kind === "local" ? (
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={usernameId}>Reserved username</Label>
                <Input
                  id={usernameId}
                  value={username}
                  maxLength={200}
                  disabled={busy}
                  onChange={(event) => setUsername(event.target.value)}
                />
              </div>
            ) : (
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={accountId}>Library account</Label>
                <Select value={libraryUserId} disabled={busy} onValueChange={setLibraryUserId}>
                  <SelectTrigger id={accountId}>
                    <SelectValue placeholder="Choose one account" />
                  </SelectTrigger>
                  <SelectContent>
                    {(candidates ?? []).map((candidate) => (
                      <SelectItem
                        key={candidate.id}
                        value={candidate.id}
                        disabled={candidate.imported || candidate.disabled}
                      >
                        {candidate.name}
                        {candidate.imported ? " · already imported" : candidate.disabled ? " · disabled" : ""}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {selectedCandidate && (
                  <p className="text-muted-foreground text-xs">
                    Media-server role: {selectedCandidate.isAdmin ? "Administrator" : "Member"}
                  </p>
                )}
              </div>
            )}

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={contactId}>Contact email (optional)</Label>
                <Input
                  id={contactId}
                  type="email"
                  value={contactEmail}
                  maxLength={320}
                  disabled={busy}
                  onChange={(event) => setContactEmail(event.target.value)}
                />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={roleId}>Initial role</Label>
                <Select
                  value={role}
                  disabled={busy}
                  onValueChange={(value) => setRole(value as CreateInvitationInputBodyRole)}
                >
                  <SelectTrigger id={roleId}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="member">Member</SelectItem>
                    <SelectItem value="admin">Administrator</SelectItem>
                  </SelectContent>
                </Select>
                <p className="text-muted-foreground text-xs">
                  Initial Loomarr role: {role === "admin" ? "Administrator" : "Member"}
                </p>
              </div>
            </div>

            {onSendEmail && (
              <Label htmlFor={emailId} className="flex min-h-11 items-center gap-3 rounded-lg border p-3">
                <Checkbox
                  id={emailId}
                  checked={sendEmail}
                  disabled={busy || !contactEmail.trim()}
                  onChange={(event) => setSendEmail(event.target.checked)}
                />
                <span>
                  <span className="block font-medium">Also send by email</span>
                  <span className="block text-muted-foreground text-xs">
                    QR and copy remain available even if email is disabled or fails.
                  </span>
                </span>
              </Label>
            )}

            {problem != null && <ErrorState error={problem} className="p-4" />}
            <DialogFooter>
              <DialogClose render={<Button type="button" variant="outline" disabled={busy} />}>
                Cancel
              </DialogClose>
              <Button type="submit" disabled={busy}>
                {busy ? "Creating…" : "Create invitation"}
              </Button>
            </DialogFooter>
          </form>
        ) : terminal ? (
          <div className="space-y-4">
            <div className="rounded-lg border border-border bg-static-900 p-5">
              <p className="font-medium">
                Invitation {invitation.status === "revoked" ? "revoked" : invitation.status}
              </p>
              <p className="mt-1 text-muted-foreground text-sm">
                This invitation can no longer issue a QR code or copied link.
              </p>
            </div>
            <DialogFooter>
              <DialogClose render={<Button type="button" />}>Done</DialogClose>
            </DialogFooter>
          </div>
        ) : grant ? (
          <div className="space-y-5">
            <div className="grid items-center gap-5 sm:grid-cols-[220px_minmax(0,1fr)]">
              <div className="mx-auto rounded-xl bg-white p-3 forced-colors:border forced-colors:border-current">
                <QrCode
                  accessibilityLabel={`Scan to accept ${invitationName(invitation)}’s Loomarr invitation`}
                  size={196}
                  value={grant.url}
                />
              </div>
              <div className="space-y-3">
                <div className="flex flex-wrap gap-2">
                  <Badge variant={invitation.role === "admin" ? "tune" : "neutral"}>
                    {invitation.role === "admin" ? "Administrator" : "Member"}
                  </Badge>
                  <Badge variant="neutral">Expires {expiryLabel(grant.expiresAt)}</Badge>
                </div>
                <p className="text-sm">
                  Ask {invitationName(invitation)} to scan this code, or copy the same private link.
                </p>
                <p className="text-muted-foreground text-xs">
                  Regenerating replaces every outstanding invitation link, including links already emailed.
                </p>
              </div>
            </div>

            {emailProblem && (
              <p role="alert" className="rounded-lg border border-destructive/50 p-3 text-sm">
                The invitation is ready, but email could not be queued. Share this QR code or copied link.
              </p>
            )}
            {problem != null && <ErrorState error={problem} className="p-4" />}

            <DialogFooter className="flex-wrap justify-between">
              <Button type="button" variant="destructive" disabled={busy} onClick={() => void revoke()}>
                <Trash2 aria-hidden /> Revoke invitation
              </Button>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="outline"
                  disabled={busy}
                  onClick={() => void issue(invitation)}
                >
                  <RefreshCw aria-hidden /> Regenerate link
                </Button>
                <Button
                  type="button"
                  disabled={busy}
                  aria-label={copied ? "Copied" : "Copy invitation link"}
                  onClick={async () => {
                    setProblem(undefined);
                    try {
                      await navigator.clipboard.writeText(grant.url);
                      setCopied(true);
                    } catch {
                      setCopied(false);
                      setProblem(
                        new Error("The invitation link could not be copied. Try again or scan the QR code."),
                      );
                    }
                  }}
                >
                  <Copy aria-hidden /> {copied ? "Copied" : "Copy link"}
                </Button>
              </div>
            </DialogFooter>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="rounded-lg border border-border bg-static-900 p-5">
              <p className="font-medium">{busy ? "Preparing a private link…" : "Ready to share"}</p>
              <p className="mt-1 text-muted-foreground text-sm">
                Generating a new link invalidates every earlier copied, QR, or email link for this invitation.
              </p>
            </div>
            {problem != null && <ErrorState error={problem} className="p-4" />}
            <DialogFooter>
              <Button type="button" disabled={busy} onClick={() => void issue(invitation)}>
                {busy ? "Generating…" : "Generate QR and link"}
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
};

export { InvitationDialog };
