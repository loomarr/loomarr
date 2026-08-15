import * as authApi from "@loomarr/api/endpoints/auth";
import * as setupApi from "@loomarr/api/endpoints/setup";
import { ApiError } from "@loomarr/api/mutator";
import { bootstrapSchema } from "@loomarr/core/schemas";
import { useForm } from "@tanstack/react-form";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Check, Loader2 } from "lucide-react";
import { useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { BootstrapStepProps } from "./bootstrap-step.type";

// Wizard step 1 — create the owning admin (§11, §13). Unauthenticated *because* it is
// gated on "no admin exists yet"; the first success closes the door (a second call 409s).
// Bootstrap issues no session, so on success we immediately sign the new admin in with
// the same credentials — the operator types their password once, not twice. Validation is
// bootstrapSchema from packages/core as a Standard Schema validator (§14); its cross-field
// refine carries path ["confirm"], so a mismatch renders on that field.
const BootstrapStep = ({ onDone, ownerName }: BootstrapStepProps) => {
  const queryClient = useQueryClient();
  const [taken, setTaken] = useState(false);

  const login = authApi.useLogin({
    mutation: {
      onSuccess: async () => {
        await queryClient.invalidateQueries({ queryKey: authApi.getMeQueryKey() });
        onDone();
      },
    },
  });

  const bootstrap = setupApi.useBootstrap({
    mutation: {
      onSuccess: (_res, vars) => {
        login.mutate({ data: { username: vars.data.username, password: vars.data.password } });
      },
      onError: (err) => {
        // 409 = an admin already exists. That isn't a failure to explain away — it just
        // means this instance is past bootstrap and the operator should sign in.
        if (err instanceof ApiError && err.status === 409) setTaken(true);
      },
    },
  });

  const form = useForm({
    defaultValues: { username: "", password: "", confirm: "" },
    validators: { onSubmit: bootstrapSchema },
    onSubmit: ({ value }) => {
      bootstrap.mutate({ data: { username: value.username, password: value.password } });
    },
  });

  const busy = bootstrap.isPending || login.isPending;
  const blockingError = taken ? undefined : (bootstrap.error ?? login.error);

  // ⚠ ALREADY DONE ⇒ SHOW THE OUTCOME, NEVER THE FORM. Bootstrap runs exactly once, so an
  // operator who walks Back to this step (or deep-links to it) was being offered a full
  // username/password/confirm form for an action guaranteed to 409 — a dead-end they could
  // only discover by filling it in and submitting. The backend was never at risk; the defect
  // was the UI advertising an impossible action.
  //
  // Keyed on the SESSION rather than on `taken`, which is only set by a failed submit: the
  // point is to never reach that submit. The owner comes from the route, which resolved the
  // same identity to pick this step, so the two cannot disagree.
  if (ownerName !== undefined) {
    return (
      <div className="flex flex-col gap-3">
        {/* ⚠ Whitespace here is CONTENT. A newline between text and an inline element is
            collapsed by JSX, so wrapping this sentence across the print width rendered as
            "Signed in as ⏎ fictional ⏎ …" with a stray gap around the name. Kept as ONE
            text node inside a <span> so the formatter cannot break it mid-sentence. */}
        <p className="flex items-center gap-2 text-sm">
          <Check className="size-4 shrink-0 text-signal" aria-hidden />
          <span>Your admin account is ready. You&rsquo;re signed in as {ownerName}.</span>
        </p>
        {/* Plain language over mechanism: "bootstrap runs once" is our word for it, not the
            operator's, and it states an internal constraint rather than what they can do. */}
        <p className="text-muted-foreground text-sm">
          There&rsquo;s only one owner account, so there&rsquo;s nothing more to do here. You can add other
          people from People once setup is finished.
        </p>
      </div>
    );
  }

  if (taken) {
    return (
      <div className="flex flex-col gap-3">
        <p className="text-sm">This Loomarr already has an owner account, so you can sign in instead.</p>
        <Link to="/login" className="w-fit text-sm text-tune underline-offset-4 hover:underline">
          Sign in instead
        </Link>
      </div>
    );
  }

  return (
    <form
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        form.handleSubmit();
      }}
      className="flex flex-col gap-4"
    >
      {blockingError != null && <ErrorState error={blockingError} />}

      <form.Field name="username">
        {(field) => (
          <div className="flex flex-col gap-2">
            <Label htmlFor="bootstrap-username">Username</Label>
            <Input
              id="bootstrap-username"
              name={field.name}
              autoComplete="username"
              value={field.state.value}
              onBlur={field.handleBlur}
              onChange={(e) => field.handleChange(e.target.value)}
            />
            {field.state.meta.errors[0] && (
              <p role="alert" className="text-onair-300 text-sm">
                {field.state.meta.errors[0].message}
              </p>
            )}
          </div>
        )}
      </form.Field>

      <form.Field name="password">
        {(field) => (
          <div className="flex flex-col gap-2">
            <Label htmlFor="bootstrap-password">Password</Label>
            <Input
              id="bootstrap-password"
              name={field.name}
              type="password"
              autoComplete="new-password"
              value={field.state.value}
              onBlur={field.handleBlur}
              onChange={(e) => field.handleChange(e.target.value)}
            />
            {field.state.meta.errors[0] && (
              <p role="alert" className="text-onair-300 text-sm">
                {field.state.meta.errors[0].message}
              </p>
            )}
          </div>
        )}
      </form.Field>

      <form.Field name="confirm">
        {(field) => (
          <div className="flex flex-col gap-2">
            <Label htmlFor="bootstrap-confirm">Confirm password</Label>
            <Input
              id="bootstrap-confirm"
              name={field.name}
              type="password"
              autoComplete="new-password"
              value={field.state.value}
              onBlur={field.handleBlur}
              onChange={(e) => field.handleChange(e.target.value)}
            />
            {field.state.meta.errors[0] && (
              <p role="alert" className="text-onair-300 text-sm">
                {field.state.meta.errors[0].message}
              </p>
            )}
          </div>
        )}
      </form.Field>

      <Button type="submit" disabled={busy} className="w-fit">
        {busy && <Loader2 className="animate-spin" aria-hidden />}
        {busy ? "Creating…" : "Create admin"}
      </Button>
    </form>
  );
};

export { BootstrapStep };
