import { zodResolver } from "@hookform/resolvers/zod";
import { loginSchema } from "@loomarr/core";
import { Loader2, LogIn, Radio } from "lucide-react";
import { useForm } from "react-hook-form";
import { ErrorState } from "@/components/loomarr";
import { Button, Input, Label } from "@/components/ui";
import { cn } from "@/lib";
import type { LoginFormProps } from "./login-form.type";

// LoginForm — the sign-in surface (§11). Local or imported media-server credentials
// land on Loomarr's own identity; the BE returns a single `invalid credentials`
// problem for a wrong password AND an un-imported user (no enumeration), so the UI
// never distinguishes them. Validation is loginSchema from packages/core (shared
// verbatim with the future mobile app, §4.3); the block-level failure renders through
// ErrorState (RFC 7807 → words, §3), field errors inline via react-hook-form.
const LoginForm = ({ onSubmit, isPending = false, error, className }: LoginFormProps) => {
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm({
    resolver: zodResolver(loginSchema),
    defaultValues: { username: "", password: "" },
  });

  return (
    <form
      onSubmit={handleSubmit((v) => onSubmit(v))}
      noValidate
      className={cn("flex w-full flex-col gap-5", className)}
    >
      <div className="flex items-center gap-2">
        <Radio className="size-5 text-signal" aria-hidden />
        <span className="font-mono font-semibold text-md tracking-tight">Loomarr</span>
      </div>

      {error != null && <ErrorState error={error} />}

      <div className="flex flex-col gap-2">
        <Label htmlFor="login-username">Username</Label>
        <Input
          id="login-username"
          autoComplete="username"
          aria-invalid={errors.username ? "true" : undefined}
          {...register("username")}
        />
        {errors.username && (
          <p className="text-onair-300 text-sm" role="alert">
            {errors.username.message}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="login-password">Password</Label>
        <Input
          id="login-password"
          type="password"
          autoComplete="current-password"
          aria-invalid={errors.password ? "true" : undefined}
          {...register("password")}
        />
        {errors.password && (
          <p className="text-onair-300 text-sm" role="alert">
            {errors.password.message}
          </p>
        )}
      </div>

      <Button type="submit" disabled={isPending}>
        {isPending ? <Loader2 className="animate-spin" aria-hidden /> : <LogIn aria-hidden />}
        {isPending ? "Signing in…" : "Sign in"}
      </Button>
    </form>
  );
};

export { LoginForm };
