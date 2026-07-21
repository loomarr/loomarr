import { loginSchema } from "@loomarr/core";
import { useForm } from "@tanstack/react-form";
import { Loader2, LogIn } from "lucide-react";
import { ErrorState } from "@/components/loomarr";
import { Button, Input, Label } from "@/components/ui";
import { cn } from "@/lib";
import type { LoginFormProps } from "./login-form.type";

// LoginForm — the sign-in surface (§11). Local or imported media-server credentials
// land on Loomarr's own identity; the BE returns a single `invalid credentials` problem
// for a wrong password AND an un-imported user (no enumeration), so the UI never
// distinguishes them. Validation is loginSchema from packages/core, handed to TanStack
// Form as a Standard Schema validator (no resolver adapter — §14) and shared verbatim
// with the future mobile app (§4.3). Validating on submit, not on change, keeps errors
// out of the operator's way while they type. The block-level failure renders through
// ErrorState (RFC 7807 → words, §3), field errors inline.
const LoginForm = ({ onSubmit, isPending = false, error, className }: LoginFormProps) => {
  const form = useForm({
    defaultValues: { username: "", password: "" },
    validators: { onSubmit: loginSchema },
    onSubmit: ({ value }) => onSubmit(value),
  });

  return (
    <form
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        form.handleSubmit();
      }}
      className={cn("flex w-full flex-col gap-5", className)}
    >
      {error != null && <ErrorState error={error} />}

      <form.Field name="username">
        {(field) => (
          <div className="flex flex-col gap-2">
            <Label htmlFor="login-username">Username</Label>
            <Input
              id="login-username"
              name={field.name}
              autoComplete="username"
              value={field.state.value}
              aria-invalid={field.state.meta.errors.length > 0 ? "true" : undefined}
              onBlur={field.handleBlur}
              onChange={(e) => field.handleChange(e.target.value)}
            />
            {field.state.meta.errors[0] && (
              <p className="text-onair-300 text-sm" role="alert">
                {field.state.meta.errors[0].message}
              </p>
            )}
          </div>
        )}
      </form.Field>

      <form.Field name="password">
        {(field) => (
          <div className="flex flex-col gap-2">
            <Label htmlFor="login-password">Password</Label>
            <Input
              id="login-password"
              name={field.name}
              type="password"
              autoComplete="current-password"
              value={field.state.value}
              aria-invalid={field.state.meta.errors.length > 0 ? "true" : undefined}
              onBlur={field.handleBlur}
              onChange={(e) => field.handleChange(e.target.value)}
            />
            {field.state.meta.errors[0] && (
              <p className="text-onair-300 text-sm" role="alert">
                {field.state.meta.errors[0].message}
              </p>
            )}
          </div>
        )}
      </form.Field>

      <Button type="submit" disabled={isPending}>
        {isPending ? <Loader2 className="animate-spin" aria-hidden /> : <LogIn aria-hidden />}
        {isPending ? "Signing in…" : "Sign in"}
      </Button>
    </form>
  );
};

export { LoginForm };
