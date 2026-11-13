import { UserBodyRole } from "@loomarr/api/models/userBodyRole";
import { toProblem } from "@loomarr/api/mutator";
import { contactAddressSchema, resetUserPasswordSchema } from "@loomarr/core/schemas";
import {
  Action,
  Field,
  FieldHint,
  Icon,
  icons,
  Stack,
  StatusLabel,
  Text,
  Toggle,
} from "@loomarr/design-system";
import { useCommittedDraft } from "@loomarr/ui";
import { useStore } from "@tanstack/react-form";
import { useId, useState } from "react";
import { BrowserSelectControl } from "@/client-adapters";
import { SessionList } from "@/components/loomarr/people/session-list";
import { useAppForm } from "@/lib/app-form";
import type { PersonDetailProps } from "./person-detail.type";

const ROLE_OPTIONS = [
  { label: "Member", value: UserBodyRole.member },
  { label: "Admin", value: UserBodyRole.admin },
];

const PersonDetail = ({
  user,
  isSelf,
  busy,
  sessions,
  sessionsLoading,
  revoking,
  onRoleChange,
  onQuotaChange,
  onToggleDisabled,
  onToggleAutoApprove,
  onRevokeSession,
  onResetPassword,
  onSetContactAddress,
  onRemoveContactAddress,
  onCancelContactReplacement,
}: PersonDetailProps) => {
  const autoId = useId();
  // The quota box saves on leaving it; undefined is untouched, with nothing to save.
  const { draft: quotaDraft, editDraft: setQuotaDraft } = useCommittedDraft<string | undefined>(undefined);
  const [resetOpen, setResetOpen] = useState(false);
  const [resetError, setResetError] = useState("");
  const [contactOpen, setContactOpen] = useState(false);
  const [contactError, setContactError] = useState("");
  const [contactSaving, setContactSaving] = useState(false);

  const resetForm = useAppForm({
    defaultValues: { next: "" },
    validators: { onSubmit: resetUserPasswordSchema },
    onSubmit: async ({ value }) => {
      if (!onResetPassword) return;
      setResetError("");
      try {
        await onResetPassword(value.next);
        resetForm.reset();
        setResetOpen(false);
      } catch (error) {
        setResetError(toProblem(error).detail ?? "Couldn't reset that password.");
      }
    },
  });
  const resetting = useStore(resetForm.store, (state) => state.isSubmitting);

  const contactForm = useAppForm({
    defaultValues: { email: "" },
    validators: { onSubmit: contactAddressSchema },
    onSubmit: async ({ value }) => {
      if (!onSetContactAddress) return;
      setContactError("");
      setContactSaving(true);
      try {
        await onSetContactAddress(contactAddressSchema.parse(value).email);
        setContactOpen(false);
      } catch (error) {
        setContactError(toProblem(error).detail ?? "Couldn't save that contact email.");
      } finally {
        setContactSaving(false);
      }
    },
  });

  const openContact = () => {
    contactForm.reset({ email: user.contactReplacement?.email ?? user.contactAddress?.email ?? "" });
    setContactError("");
    setContactOpen(true);
  };

  const runContactAction = async (action: (() => Promise<void>) | undefined) => {
    if (!action) return;
    setContactError("");
    setContactSaving(true);
    try {
      await action();
    } catch (error) {
      setContactError(toProblem(error).detail ?? "Couldn't update that contact email.");
    } finally {
      setContactSaving(false);
    }
  };

  return (
    <Stack className="p-6" gap="section">
      <Stack aria-labelledby="person-identity" gap="inline" render="section">
        <Text as="h3" id="person-identity" textRole="heading">
          Identity
        </Text>
        <Stack align="center" direction="row" gap="inline" wrap>
          {isSelf && <StatusLabel tone="notice">You</StatusLabel>}
          {user.disabled && <StatusLabel tone="error">Disabled</StatusLabel>}
        </Stack>
        <Stack align="center" className="text-muted-foreground" direction="row" gap="inline" render="p">
          {user.local ? (
            <Icon decorative glyph={icons.key} size="compact" tone="inherit" />
          ) : (
            <Icon decorative glyph={icons.server} size="compact" tone="inherit" />
          )}
          <Text as="span" textRole="compact" tone="secondary">
            {user.local
              ? "Loomarr-local credentials"
              : user.offlineLogin
                ? "Media-server credentials · offline login ready"
                : "Media-server credentials · sign in once to enable offline login"}
          </Text>
        </Stack>
      </Stack>

      <Stack
        aria-labelledby="person-contact"
        className="border-static-800 border-t pt-6"
        gap="control"
        render="section"
      >
        <div>
          <Text as="h3" id="person-contact" textRole="heading">
            Contact
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            Optional account messages only. Email never replaces this person's sign-in name.
          </Text>
        </div>

        {user.contactAddress ? (
          <Stack className="border-static-800 rounded-md border p-3" gap="inline">
            <Stack align="center" direction="row" gap="inline" wrap>
              <Icon decorative glyph={icons.mail} size="compact" tone="muted" />
              <Text as="span" className="break-all" textRole="compact">
                {user.contactAddress.email}
              </Text>
              <StatusLabel tone={user.contactAddress.status === "verified" ? "ok" : "off"}>
                {user.contactAddress.status === "verified" ? "Verified" : "Unverified"}
              </StatusLabel>
            </Stack>
            <Text as="p" textRole="hint" tone="secondary">
              {user.contactAddress.status === "verified"
                ? "Verified for local-password recovery."
                : "Not recovery-capable until the person verifies possession."}
            </Text>
          </Stack>
        ) : (
          <Text as="p" textRole="compact" tone="secondary">
            No contact email. QR and direct account access still work.
          </Text>
        )}

        {user.contactReplacement && (
          <Stack className="border-suggest-700/50 bg-suggest-950/20 rounded-md border p-3" gap="inline">
            <Stack align="center" direction="row" gap="inline" wrap>
              <Text as="span" className="break-all" textRole="compact">
                {user.contactReplacement.email}
              </Text>
              <StatusLabel tone="pending">Pending replacement</StatusLabel>
            </Stack>
            <Text as="p" textRole="hint" tone="secondary">
              The verified address above remains recovery-capable until this replacement is verified.
            </Text>
            <Action
              disabled={contactSaving}
              onPress={() => void runContactAction(onCancelContactReplacement)}
              style={{ alignSelf: "flex-start" }}
              variant="ghost"
            >
              Cancel replacement
            </Action>
          </Stack>
        )}

        {contactOpen ? (
          <form
            className="flex flex-col gap-3"
            noValidate
            onSubmit={(event) => {
              event.preventDefault();
              void contactForm.handleSubmit();
            }}
          >
            <contactForm.AppField name="email">
              {(field) => <field.TextField autoComplete="off" label="Contact email" type="email" />}
            </contactForm.AppField>
            <Text as="p" textRole="hint" tone="secondary">
              Saving creates an unverified address. Invitation or verification delivery will prove possession.
            </Text>
            {contactError && (
              <FieldHint role="alert" tone="error">
                {contactError}
              </FieldHint>
            )}
            <Stack direction="row" gap="inline" wrap>
              <Action disabled={contactSaving} submit>
                Save email
              </Action>
              <Action
                onPress={() => {
                  setContactOpen(false);
                  contactForm.reset({ email: "" });
                  setContactError("");
                }}
                variant="ghost"
              >
                Cancel
              </Action>
            </Stack>
          </form>
        ) : (
          <Stack direction="row" gap="inline" wrap>
            <Action disabled={contactSaving} onPress={openContact} variant="outline">
              {user.contactAddress ? "Change email" : "Add email"}
            </Action>
            {user.contactAddress && (
              <Action
                disabled={contactSaving}
                onPress={() => void runContactAction(onRemoveContactAddress)}
                variant="ghost"
              >
                Remove email
              </Action>
            )}
          </Stack>
        )}
        {!contactOpen && contactError && (
          <FieldHint role="alert" tone="error">
            {contactError}
          </FieldHint>
        )}
      </Stack>

      <Stack
        aria-labelledby="person-access"
        className="border-static-800 border-t pt-6"
        gap="control"
        render="section"
      >
        <div>
          <Text as="h3" id="person-access" textRole="heading">
            Access
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            Role changes take effect immediately.
          </Text>
        </div>
        <Stack gap="1.5">
          <BrowserSelectControl
            appearance="field"
            disabled={busy || isSelf}
            label="Role"
            onValueChange={(value) => onRoleChange?.(value)}
            options={ROLE_OPTIONS}
            value={user.role}
          />
          {isSelf && <FieldHint>You cannot change your own role.</FieldHint>}
        </Stack>
      </Stack>

      <Stack
        aria-labelledby="person-requests"
        className="border-static-800 border-t pt-6"
        gap="4"
        render="section"
      >
        <div>
          <Text as="h3" id="person-requests" textRole="heading">
            Requests
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            Control how much this person may acquire and approve.
          </Text>
        </div>
        <div className="grid grid-cols-[minmax(0,1fr)_7rem] items-end gap-4">
          <div>
            <Text as="p" textRole="compact">
              Pending acquisitions
            </Text>
            <p className="text-muted-foreground font-mono text-sm tabular-nums">{`${user.pendingAcquisitions} of ${user.effectiveQuota}`}</p>
          </div>
          <Field
            key={`${user.id}-${user.quota}`}
            defaultValue={String(user.quota)}
            disabled={busy}
            keyboardType="numeric"
            label="Quota"
            placeholder={String(user.effectiveQuota)}
            onChangeText={setQuotaDraft}
            onBlur={() => {
              // Saved on leaving the box, not per keystroke; untouched, there is nothing to save.
              if (quotaDraft === undefined) return;
              const next = Number(quotaDraft);
              setQuotaDraft(undefined);
              if (Number.isInteger(next) && next >= 0 && next !== user.quota) onQuotaChange?.(next);
            }}
          />
        </div>
        <Toggle
          id={autoId}
          label="Auto-approve requests"
          checked={user.autoApprove}
          disabled={busy}
          onCheckedChange={(checked) => onToggleAutoApprove?.(checked)}
        />
      </Stack>

      <Stack
        aria-labelledby="person-sessions"
        className="border-static-800 border-t pt-6"
        gap="control"
        render="section"
      >
        <div>
          <Text as="h3" id="person-sessions" textRole="heading">
            Sessions
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            Review and end active sign-ins.
          </Text>
        </div>
        <SessionList
          userName={user.name}
          sessions={sessions}
          loading={sessionsLoading}
          revoking={revoking}
          onRevoke={onRevokeSession}
        />
      </Stack>

      <Stack
        aria-labelledby="person-password"
        className="border-static-800 border-t pt-6"
        gap="control"
        render="section"
      >
        <div>
          <Text as="h3" id="person-password" textRole="heading">
            Password
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            {user.local
              ? "Set a replacement Loomarr password and end every session."
              : "Change this password in the connected media server. Loomarr stores a non-reversible Argon2id verifier for offline login and refreshes it after a successful provider sign-in."}
          </Text>
        </div>
        {user.local &&
          onResetPassword &&
          (resetOpen ? (
            <form
              className="flex flex-col gap-3"
              noValidate
              onSubmit={(event) => {
                event.preventDefault();
                void resetForm.handleSubmit();
              }}
            >
              <resetForm.AppField name="next">
                {(field) => (
                  <field.TextField autoComplete="new-password" label="New password" type="password" />
                )}
              </resetForm.AppField>
              {resetError && (
                <FieldHint role="alert" tone="error">
                  {resetError}
                </FieldHint>
              )}
              <Stack direction="row" gap="inline" wrap>
                <Action disabled={resetting} submit>
                  Set new password
                </Action>
                <Action
                  onPress={() => {
                    setResetOpen(false);
                    resetForm.reset();
                    setResetError("");
                  }}
                  variant="ghost"
                >
                  Cancel
                </Action>
              </Stack>
            </form>
          ) : (
            <Action onPress={() => setResetOpen(true)} style={{ alignSelf: "flex-start" }} variant="outline">
              Reset password
            </Action>
          ))}
      </Stack>

      <Stack
        aria-labelledby="person-danger"
        className="border-onair-700 bg-onair-950/20 rounded-lg border p-4"
        gap="control"
        render="section"
      >
        <div>
          <Text as="h3" id="person-danger" textRole="heading" tone="danger">
            Danger zone
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            {user.disabled
              ? "Enable this account so the person can sign in again."
              : "Disable this account and end every active session immediately."}
          </Text>
        </div>
        <Action
          disabled={busy || isSelf}
          onPress={() => onToggleDisabled?.(!user.disabled)}
          style={{ alignSelf: "flex-start" }}
          tone={user.disabled ? undefined : "danger"}
          variant="outline"
        >
          {user.disabled ? "Enable account" : "Disable account"}
        </Action>
        {isSelf && (
          <Text as="p" textRole="hint" tone="secondary">
            You cannot disable your own account.
          </Text>
        )}
      </Stack>
    </Stack>
  );
};

export { PersonDetail };
