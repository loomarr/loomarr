import type { CreateInvitationInputBodyKind } from "@loomarr/api/models/createInvitationInputBodyKind";
import type { CreateInvitationInputBodyRole } from "@loomarr/api/models/createInvitationInputBodyRole";
import type { InvitationBody } from "@loomarr/api/models/invitationBody";
import type { IssueInvitationGrantOutputBody } from "@loomarr/api/models/issueInvitationGrantOutputBody";
import { createInvitationSchema } from "@loomarr/core/schemas";
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
import { QrCode } from "@loomarr/design-system/qr-code-web";
import { useForm, useStore } from "@tanstack/react-form";
import { Copy, RefreshCw, Trash2, UserPlus } from "lucide-react";
import { useId, useRef, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { FieldError } from "@/components/loomarr/feedback/field-error";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
  const [invitation, setInvitation] = useState<InvitationBody | undefined>(existing);
  const [grant, setGrant] = useState<IssueInvitationGrantOutputBody>();
  const [copied, setCopied] = useState(false);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<unknown>();
  const [emailProblem, setEmailProblem] = useState(false);
  const open = controlledOpen ?? internalOpen;

  const form = useForm({
    defaultValues: {
      kind: "local" as CreateInvitationInputBodyKind,
      username: "",
      libraryUserId: "",
      contactEmail: "",
      role: "member" as CreateInvitationInputBodyRole,
      sendEmail: false,
    },
    validators: { onSubmit: createInvitationSchema },
    onSubmit: async ({ value }) => {
      const current = operation.current;
      const input = createInvitationSchema.parse(value);
      setBusy(true);
      try {
        const created = await onCreate({
          kind: input.kind,
          ...(input.kind === "local" ? { username: input.username } : { libraryUserId: input.libraryUserId }),
          contactEmail: input.contactEmail,
          role: input.role,
        });
        if (operation.current !== current) return;
        setInvitation(created);
        const issued = await onIssueGrant(created.id, "qr");
        if (operation.current !== current) return;
        setGrant(issued);
        if (value.sendEmail && input.contactEmail && onSendEmail) {
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
    },
  });
  const kind = useStore(form.store, (state) => state.values.kind);
  const contactEmail = useStore(form.store, (state) => state.values.contactEmail);

  const reset = () => {
    operation.current += 1;
    form.reset();
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

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    setProblem(undefined);
    setEmailProblem(false);
    void form.handleSubmit();
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

  const terminal = invitation?.status !== undefined && invitation.status !== "pending";
  const title = existing ? `Share ${invitationName(existing)}’s invitation` : "Invite person";

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      {controlledOpen === undefined && (
        <DialogTrigger>
          <Button>
            <UserPlus aria-hidden /> Invite person
          </Button>
        </DialogTrigger>
      )}
      <DialogContent portalContainer={portalContainer} size="form">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            Reserve access, then share a short-lived link. Creating an invitation does not activate an
            account.
          </DialogDescription>
        </DialogHeader>

        {!invitation ? (
          <form className="flex flex-col gap-4" noValidate onSubmit={submit}>
            <form.Field name="kind">
              {(field) => (
                <fieldset className="grid gap-2 sm:grid-cols-2">
                  <legend className="mb-2 text-sm font-medium">Credential path</legend>
                  <Label className="border-border flex min-h-11 cursor-pointer items-center gap-3 rounded-lg border p-3">
                    <input
                      type="radio"
                      name="invitation-kind"
                      value="local"
                      checked={field.state.value === "local"}
                      disabled={busy}
                      onChange={() => field.handleChange("local")}
                    />
                    Local Loomarr account
                  </Label>
                  <Label className="border-border flex min-h-11 cursor-pointer items-center gap-3 rounded-lg border p-3">
                    <input
                      type="radio"
                      name="invitation-kind"
                      value="library"
                      checked={field.state.value === "library"}
                      disabled={busy || !libraryAvailable}
                      onChange={() => field.handleChange("library")}
                    />
                    Emby or Jellyfin account
                  </Label>
                </fieldset>
              )}
            </form.Field>

            {kind === "local" ? (
              <form.Field name="username">
                {(field) => (
                  <div className="flex flex-col gap-1.5">
                    <Label htmlFor={usernameId}>Reserved username</Label>
                    <Input
                      id={usernameId}
                      name={field.name}
                      value={field.state.value}
                      maxLength={200}
                      disabled={busy}
                      aria-invalid={field.state.meta.errors.length > 0 ? "true" : undefined}
                      onChange={(event) => field.handleChange(event.target.value)}
                    />
                    <FieldError errors={field.state.meta.errors} />
                  </div>
                )}
              </form.Field>
            ) : (
              <form.Field name="libraryUserId">
                {(field) => {
                  const selectedCandidate = candidates?.find(
                    (candidate) => candidate.id === field.state.value,
                  );
                  return (
                    <div className="flex flex-col gap-1.5">
                      <Label htmlFor={accountId}>Library account</Label>
                      <Select
                        value={field.state.value}
                        disabled={busy}
                        onValueChange={(value) => field.handleChange(value)}
                      >
                        <SelectTrigger
                          id={accountId}
                          aria-invalid={field.state.meta.errors.length > 0 ? "true" : undefined}
                        >
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
                              {candidate.imported
                                ? " · already imported"
                                : candidate.disabled
                                  ? " · disabled"
                                  : ""}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      {selectedCandidate && (
                        <p className="text-muted-foreground text-xs">
                          Media-server role: {selectedCandidate.isAdmin ? "Administrator" : "Member"}
                        </p>
                      )}
                      <FieldError errors={field.state.meta.errors} />
                    </div>
                  );
                }}
              </form.Field>
            )}

            <div className="grid gap-4 sm:grid-cols-2">
              <form.Field name="contactEmail">
                {(field) => (
                  <div className="flex flex-col gap-1.5">
                    <Label htmlFor={contactId}>Contact email (optional)</Label>
                    <Input
                      id={contactId}
                      name={field.name}
                      type="email"
                      value={field.state.value}
                      maxLength={320}
                      disabled={busy}
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
                    <Label htmlFor={roleId}>Initial role</Label>
                    <Select
                      value={field.state.value}
                      disabled={busy}
                      onValueChange={(value) => field.handleChange(value as CreateInvitationInputBodyRole)}
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
                      Initial Loomarr role: {field.state.value === "admin" ? "Administrator" : "Member"}
                    </p>
                  </div>
                )}
              </form.Field>
            </div>

            {onSendEmail && (
              <form.Field name="sendEmail">
                {(field) => (
                  <Label htmlFor={emailId} className="flex min-h-11 items-center gap-3 rounded-lg border p-3">
                    <Checkbox
                      id={emailId}
                      checked={field.state.value}
                      disabled={busy || !contactEmail.trim()}
                      onChange={(event) => field.handleChange(event.target.checked)}
                    />
                    <span>
                      <span className="block font-medium">Also send by email</span>
                      <span className="text-muted-foreground block text-xs">
                        QR and copy remain available even if email is disabled or fails.
                      </span>
                    </span>
                  </Label>
                )}
              </form.Field>
            )}

            {problem != null && <ErrorState error={problem} className="p-4" />}
            <DialogFooter>
              <DialogClose>
                <Button type="button" variant="outline" disabled={busy}>
                  Cancel
                </Button>
              </DialogClose>
              <Button type="submit" disabled={busy}>
                {busy ? "Creating…" : "Create invitation"}
              </Button>
            </DialogFooter>
          </form>
        ) : terminal ? (
          <div className="space-y-4">
            <div className="border-border bg-static-900 rounded-lg border p-5">
              <p className="font-medium">
                Invitation {invitation.status === "revoked" ? "revoked" : invitation.status}
              </p>
              <p className="text-muted-foreground mt-1 text-sm">
                This invitation can no longer issue a QR code or copied link.
              </p>
            </div>
            <DialogFooter>
              <DialogClose>
                <Button type="button">Done</Button>
              </DialogClose>
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
              <p role="alert" className="border-destructive/50 rounded-lg border p-3 text-sm">
                The invitation is ready, but email could not be queued. Share this QR code or copied link.
              </p>
            )}
            {problem != null && <ErrorState error={problem} className="p-4" />}

            <DialogFooter spread>
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
                  aria-label={copied ? "Copied" : "Copy link to this invitation"}
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
            <div className="border-border bg-static-900 rounded-lg border p-5">
              <p className="font-medium">{busy ? "Preparing a private link…" : "Ready to share"}</p>
              <p className="text-muted-foreground mt-1 text-sm">
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
