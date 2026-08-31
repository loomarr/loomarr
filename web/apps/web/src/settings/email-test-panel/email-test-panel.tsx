import * as notificationsApi from "@loomarr/api/endpoints/notifications";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useSettingsEdits } from "@/settings/settings-edits";

const EmailTestPanel = () => {
  const [destination, setDestination] = useState("");
  const { edits } = useSettingsEdits();
  const hasUnsavedChanges = Object.keys(edits).length > 0;
  const send = notificationsApi.useNotificationsEmailTest();
  const result = send.data?.status === 200 ? send.data.data : undefined;

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    const to = destination.trim();
    if (!to || hasUnsavedChanges) return;
    send.mutate({ data: { to } });
  };

  return (
    <section className="rounded-lg border border-border p-4">
      <h2 className="font-medium text-sm">Test email delivery</h2>
      <p className="mt-1 text-muted-foreground text-sm leading-relaxed">
        Sends a harmless message through the settings currently applied by Loomarr. Save changes first so the
        test checks exactly what is shown above.
      </p>
      <form className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-end" onSubmit={submit}>
        <label className="min-w-0 flex-1 text-sm" htmlFor="email-test-recipient">
          <span className="mb-1.5 block font-medium">Test recipient</span>
          <Input
            id="email-test-recipient"
            autoComplete="email"
            inputMode="email"
            onChange={(event) => setDestination(event.target.value)}
            placeholder="admin@example.com"
            type="email"
            value={destination}
          />
        </label>
        <Button disabled={!destination.trim() || hasUnsavedChanges || send.isPending} type="submit">
          {send.isPending ? "Sending…" : "Send test email"}
        </Button>
      </form>

      {hasUnsavedChanges ? (
        <p className="mt-3 text-muted-foreground text-sm">
          Save your pending settings before sending a test.
        </p>
      ) : null}
      {result ? (
        <p
          aria-live="polite"
          className={result.ok ? "mt-3 text-sm" : "mt-3 text-destructive text-sm"}
          role="status"
        >
          {result.hint ?? (result.ok ? "Test message sent." : "The test message could not be sent.")}
        </p>
      ) : null}
      {send.isError ? (
        <p className="mt-3 text-destructive text-sm" role="alert">
          Loomarr could not run the email test. Try again after checking the saved settings.
        </p>
      ) : null}
    </section>
  );
};

export { EmailTestPanel };
