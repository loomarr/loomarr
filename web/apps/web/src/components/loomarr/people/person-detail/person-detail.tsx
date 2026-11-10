import { UserBodyRole } from "@loomarr/api/models/userBodyRole";
import { toProblem } from "@loomarr/api/mutator";
import { contactAddressSchema, resetUserPasswordSchema } from "@loomarr/core/schemas";
import { Action, Field, FieldHint, Icon, icons, StatusLabel, Toggle } from "@loomarr/design-system";
import { useStore } from "@tanstack/react-form";
import { useId, useRef, useState } from "react";
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
  const quotaDraft = useRef<string>(undefined);
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
    <div className="flex flex-col gap-6 p-6">
      <section aria-labelledby="person-identity" className="flex flex-col gap-2">
        <h3 id="person-identity" className="font-semibold">
          Identity
        </h3>
        <div className="flex flex-wrap items-center gap-2">
          {isSelf && <StatusLabel tone="notice">You</StatusLabel>}
          {user.disabled && <StatusLabel tone="error">Disabled</StatusLabel>}
        </div>
        <p className="text-muted-foreground flex items-center gap-2 text-sm">
          {user.local ? (
            <Icon decorative glyph={icons.key} size="compact" tone="inherit" />
          ) : (
            <Icon decorative glyph={icons.server} size="compact" tone="inherit" />
          )}
          {user.local
            ? "Loomarr-local credentials"
            : user.offlineLogin
              ? "Media-server credentials · offline login ready"
              : "Media-server credentials · sign in once to enable offline login"}
        </p>
      </section>

      <section
        aria-labelledby="person-contact"
        className="border-static-800 flex flex-col gap-3 border-t pt-6"
      >
        <div>
          <h3 id="person-contact" className="font-semibold">
            Contact
          </h3>
          <p className="text-muted-foreground text-sm">
            Optional account messages only. Email never replaces this person's sign-in name.
          </p>
        </div>

        {user.contactAddress ? (
          <div className="border-static-800 flex flex-col gap-2 rounded-md border p-3">
            <div className="flex flex-wrap items-center gap-2">
              <Icon decorative glyph={icons.mail} size="compact" tone="muted" />
              <span className="text-sm break-all">{user.contactAddress.email}</span>
              <StatusLabel tone={user.contactAddress.status === "verified" ? "ok" : "off"}>
                {user.contactAddress.status === "verified" ? "Verified" : "Unverified"}
              </StatusLabel>
            </div>
            <p className="text-muted-foreground text-xs">
              {user.contactAddress.status === "verified"
                ? "Verified for local-password recovery."
                : "Not recovery-capable until the person verifies possession."}
            </p>
          </div>
        ) : (
          <p className="text-muted-foreground text-sm">
            No contact email. QR and direct account access still work.
          </p>
        )}

        {user.contactReplacement && (
          <div className="border-suggest-700/50 bg-suggest-950/20 flex flex-col gap-2 rounded-md border p-3">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm break-all">{user.contactReplacement.email}</span>
              <StatusLabel tone="pending">Pending replacement</StatusLabel>
            </div>
            <p className="text-muted-foreground text-xs">
              The verified address above remains recovery-capable until this replacement is verified.
            </p>
            <Action
              disabled={contactSaving}
              onPress={() => void runContactAction(onCancelContactReplacement)}
              style={{ alignSelf: "flex-start" }}
              variant="ghost"
            >
              Cancel replacement
            </Action>
          </div>
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
            <p className="text-muted-foreground text-xs">
              Saving creates an unverified address. Invitation or verification delivery will prove possession.
            </p>
            {contactError && (
              <FieldHint role="alert" tone="error">
                {contactError}
              </FieldHint>
            )}
            <div className="flex flex-wrap gap-2">
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
            </div>
          </form>
        ) : (
          <div className="flex flex-wrap gap-2">
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
          </div>
        )}
        {!contactOpen && contactError && (
          <FieldHint role="alert" tone="error">
            {contactError}
          </FieldHint>
        )}
      </section>

      <section
        aria-labelledby="person-access"
        className="border-static-800 flex flex-col gap-3 border-t pt-6"
      >
        <div>
          <h3 id="person-access" className="font-semibold">
            Access
          </h3>
          <p className="text-muted-foreground text-sm">Role changes take effect immediately.</p>
        </div>
        <div className="flex flex-col gap-1.5">
          <BrowserSelectControl
            appearance="field"
            disabled={busy || isSelf}
            label="Role"
            onValueChange={(value) => onRoleChange?.(value)}
            options={ROLE_OPTIONS}
            value={user.role}
          />
          {isSelf && <FieldHint>You cannot change your own role.</FieldHint>}
        </div>
      </section>

      <section
        aria-labelledby="person-requests"
        className="border-static-800 flex flex-col gap-4 border-t pt-6"
      >
        <div>
          <h3 id="person-requests" className="font-semibold">
            Requests
          </h3>
          <p className="text-muted-foreground text-sm">
            Control how much this person may acquire and approve.
          </p>
        </div>
        <div className="grid grid-cols-[minmax(0,1fr)_7rem] items-end gap-4">
          <div>
            <p className="text-sm">Pending acquisitions</p>
            <p className="text-muted-foreground font-mono text-sm tabular-nums">{`${user.pendingAcquisitions} of ${user.effectiveQuota}`}</p>
          </div>
          <Field
            key={`${user.id}-${user.quota}`}
            defaultValue={String(user.quota)}
            disabled={busy}
            keyboardType="numeric"
            label="Quota"
            placeholder={String(user.effectiveQuota)}
            onChangeText={(text) => {
              quotaDraft.current = text;
            }}
            onBlur={() => {
              // Saved on leaving the box, not per keystroke; untouched, there is nothing to save.
              if (quotaDraft.current === undefined) return;
              const next = Number(quotaDraft.current);
              quotaDraft.current = undefined;
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
      </section>

      <section
        aria-labelledby="person-sessions"
        className="border-static-800 flex flex-col gap-3 border-t pt-6"
      >
        <div>
          <h3 id="person-sessions" className="font-semibold">
            Sessions
          </h3>
          <p className="text-muted-foreground text-sm">Review and end active sign-ins.</p>
        </div>
        <SessionList
          userName={user.name}
          sessions={sessions}
          loading={sessionsLoading}
          revoking={revoking}
          onRevoke={onRevokeSession}
        />
      </section>

      <section
        aria-labelledby="person-password"
        className="border-static-800 flex flex-col gap-3 border-t pt-6"
      >
        <div>
          <h3 id="person-password" className="font-semibold">
            Password
          </h3>
          <p className="text-muted-foreground text-sm">
            {user.local
              ? "Set a replacement Loomarr password and end every session."
              : "Change this password in the connected media server. Loomarr stores a non-reversible Argon2id verifier for offline login and refreshes it after a successful provider sign-in."}
          </p>
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
              <div className="flex flex-wrap gap-2">
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
              </div>
            </form>
          ) : (
            <Action onPress={() => setResetOpen(true)} style={{ alignSelf: "flex-start" }} variant="outline">
              Reset password
            </Action>
          ))}
      </section>

      <section
        aria-labelledby="person-danger"
        className="border-onair-700 bg-onair-950/20 flex flex-col gap-3 rounded-lg border p-4"
      >
        <div>
          <h3 id="person-danger" className="text-onair-300 font-semibold">
            Danger zone
          </h3>
          <p className="text-muted-foreground text-sm">
            {user.disabled
              ? "Enable this account so the person can sign in again."
              : "Disable this account and end every active session immediately."}
          </p>
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
        {isSelf && <p className="text-static-400 text-xs">You cannot disable your own account.</p>}
      </section>
    </div>
  );
};

export { PersonDetail };
