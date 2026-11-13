import { CreateInvitationInputBodyKind } from "@loomarr/api/models/createInvitationInputBodyKind";
import { CreateInvitationInputBodyRole } from "@loomarr/api/models/createInvitationInputBodyRole";
import type { InvitationBody } from "@loomarr/api/models/invitationBody";
import type { IssueInvitationGrantOutputBody } from "@loomarr/api/models/issueInvitationGrantOutputBody";
import { createInvitationSchema } from "@loomarr/core/schemas";
import {
  Action,
  Card,
  ChoiceGroup,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  FieldHint,
  Stack,
  StatusLabel,
  Text,
  Toggle,
} from "@loomarr/design-system";
import { QrCode } from "@loomarr/design-system/qr-code-web";
import { useStore } from "@tanstack/react-form";
import { useId, useRef, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useAppForm } from "@/lib/app-form";
import type { InvitationDialogProps } from "./invitation-dialog.type";

const ROLE_OPTIONS = [
  { label: "Member", value: CreateInvitationInputBodyRole.member },
  { label: "Administrator", value: CreateInvitationInputBodyRole.admin },
] as const;

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

  const form = useAppForm({
    defaultValues: {
      kind: "local" as CreateInvitationInputBodyKind,
      username: "",
      libraryUserId: "",
      contactEmail: "",
      role: CreateInvitationInputBodyRole.member as CreateInvitationInputBodyRole,
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

  // The dialog opens from its own state rather than a trigger part, so the opener can be an Action;
  // the Dialog returns focus to it on close all the same.
  return (
    <>
      {controlledOpen === undefined && <Action onPress={() => changeOpen(true)}>Invite person</Action>}
      <Dialog open={open} onOpenChange={changeOpen}>
        <DialogContent portalContainer={portalContainer} size="form">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>
              Reserve access, then share a short-lived link. Creating an invitation does not activate an
              account.
            </DialogDescription>
          </DialogHeader>

          {!invitation ? (
            <Stack gap="4" noValidate onSubmit={submit} render="form">
              <form.Field name="kind">
                {(field) => (
                  <ChoiceGroup
                    label="Credential path"
                    onValueChange={field.handleChange}
                    options={[
                      {
                        disabled: busy,
                        label: "Local Loomarr account",
                        value: CreateInvitationInputBodyKind.local,
                      },
                      {
                        disabled: busy || !libraryAvailable,
                        label: "Emby or Jellyfin account",
                        value: CreateInvitationInputBodyKind.library,
                      },
                    ]}
                    value={field.state.value}
                  />
                )}
              </form.Field>

              {kind === "local" ? (
                <form.AppField name="username">
                  {(field) => (
                    <field.TextField
                      autoComplete="off"
                      disabled={busy}
                      label="Reserved username"
                      maxLength={200}
                    />
                  )}
                </form.AppField>
              ) : (
                <form.AppField name="libraryUserId">
                  {(field) => {
                    const selectedCandidate = candidates?.find(
                      (candidate) => candidate.id === field.state.value,
                    );
                    return (
                      <Stack gap="1.5">
                        <field.SelectField
                          disabled={busy}
                          label="Library account"
                          options={(candidates ?? []).map((candidate) => ({
                            disabled: candidate.imported || candidate.disabled,
                            label: `${candidate.name}${
                              candidate.imported
                                ? " · already imported"
                                : candidate.disabled
                                  ? " · disabled"
                                  : ""
                            }`,
                            value: candidate.id,
                          }))}
                          placeholder="Choose one account"
                        />
                        {selectedCandidate && (
                          <FieldHint>
                            Media-server role: {selectedCandidate.isAdmin ? "Administrator" : "Member"}
                          </FieldHint>
                        )}
                      </Stack>
                    );
                  }}
                </form.AppField>
              )}

              <div className="grid gap-4 sm:grid-cols-2">
                <form.AppField name="contactEmail">
                  {(field) => (
                    <field.TextField
                      autoComplete="email"
                      disabled={busy}
                      label="Contact email (optional)"
                      maxLength={320}
                      type="email"
                    />
                  )}
                </form.AppField>
                <form.AppField name="role">
                  {(field) => (
                    <Stack gap="1.5">
                      <field.SelectField disabled={busy} label="Initial role" options={ROLE_OPTIONS} />
                      <FieldHint>
                        Initial Loomarr role:{" "}
                        {field.state.value === CreateInvitationInputBodyRole.admin
                          ? "Administrator"
                          : "Member"}
                      </FieldHint>
                    </Stack>
                  )}
                </form.AppField>
              </div>

              {onSendEmail && (
                <form.Field name="sendEmail">
                  {(field) => (
                    <Card paddingHorizontal={12} paddingVertical={4}>
                      <Toggle
                        id={emailId}
                        label="Also send by email"
                        description="QR and copy remain available even if email is disabled or fails."
                        checked={field.state.value}
                        disabled={busy || !contactEmail.trim()}
                        onCheckedChange={field.handleChange}
                      />
                    </Card>
                  )}
                </form.Field>
              )}

              {problem != null && <ErrorState error={problem} className="p-4" />}
              <DialogFooter>
                <Action disabled={busy} onPress={() => changeOpen(false)} variant="outline">
                  Cancel
                </Action>
                <Action disabled={busy} submit>
                  {busy ? "Creating…" : "Create invitation"}
                </Action>
              </DialogFooter>
            </Stack>
          ) : terminal ? (
            <Stack gap="4">
              <Card gap={4} padding={20}>
                <Text as="p" textRole="cardLabel">
                  Invitation {invitation.status === "revoked" ? "revoked" : invitation.status}
                </Text>
                <Text as="p" textRole="compact" tone="secondary">
                  This invitation can no longer issue a QR code or copied link.
                </Text>
              </Card>
              <DialogFooter>
                <Action onPress={() => changeOpen(false)}>Done</Action>
              </DialogFooter>
            </Stack>
          ) : grant ? (
            <Stack gap="5">
              <div className="grid items-center gap-5 sm:grid-cols-[220px_minmax(0,1fr)]">
                <div className="mx-auto rounded-xl bg-white p-3 forced-colors:border forced-colors:border-current">
                  <QrCode
                    accessibilityLabel={`Scan to accept ${invitationName(invitation)}’s Loomarr invitation`}
                    size={196}
                    value={grant.url}
                  />
                </div>
                <Stack gap="3">
                  <Stack direction="row" gap="inline" wrap>
                    <StatusLabel tone={invitation.role === "admin" ? "notice" : "off"}>
                      {invitation.role === "admin" ? "Administrator" : "Member"}
                    </StatusLabel>
                    <StatusLabel tone="pending">Expires {expiryLabel(grant.expiresAt)}</StatusLabel>
                  </Stack>
                  <Text as="p" textRole="compact">
                    Ask {invitationName(invitation)} to scan this code, or copy the same private link.
                  </Text>
                  <Text as="p" textRole="hint" tone="secondary">
                    Regenerating replaces every outstanding invitation link, including links already emailed.
                  </Text>
                </Stack>
              </div>

              {emailProblem && (
                <Stack borderRadius={12} className="border-destructive/50 border" padding={12} role="alert">
                  <Text as="p" textRole="compact">
                    The invitation is ready, but email could not be queued. Share this QR code or copied link.
                  </Text>
                </Stack>
              )}
              {problem != null && <ErrorState error={problem} className="p-4" />}

              <DialogFooter spread>
                <Action
                  disabled={busy}
                  icon="delete"
                  onPress={() => void revoke()}
                  tone="danger"
                  variant="outline"
                >
                  Revoke invitation
                </Action>
                <Stack direction="row" gap="inline" wrap>
                  <Action
                    disabled={busy}
                    icon="refresh"
                    onPress={() => void issue(invitation)}
                    variant="outline"
                  >
                    Regenerate link
                  </Action>
                  <Action
                    accessibilityLabel={copied ? "Copied" : "Copy link to this invitation"}
                    disabled={busy}
                    icon="copy"
                    onPress={async () => {
                      setProblem(undefined);
                      try {
                        await navigator.clipboard.writeText(grant.url);
                        setCopied(true);
                      } catch {
                        setCopied(false);
                        setProblem(
                          new Error(
                            "The invitation link could not be copied. Try again or scan the QR code.",
                          ),
                        );
                      }
                    }}
                  >
                    {copied ? "Copied" : "Copy link"}
                  </Action>
                </Stack>
              </DialogFooter>
            </Stack>
          ) : (
            <Stack gap="4">
              <Card gap={4} padding={20}>
                <Text as="p" textRole="cardLabel">
                  {busy ? "Preparing a private link…" : "Ready to share"}
                </Text>
                <Text as="p" textRole="compact" tone="secondary">
                  Generating a new link invalidates every earlier copied, QR, or email link for this
                  invitation.
                </Text>
              </Card>
              {problem != null && <ErrorState error={problem} className="p-4" />}
              <DialogFooter>
                <Action disabled={busy} onPress={() => void issue(invitation)}>
                  {busy ? "Generating…" : "Generate QR and link"}
                </Action>
              </DialogFooter>
            </Stack>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
};

export { InvitationDialog };
