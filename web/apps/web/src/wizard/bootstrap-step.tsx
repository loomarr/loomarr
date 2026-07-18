import { zodResolver } from "@hookform/resolvers/zod";
import { ApiError, authApi, setupApi } from "@loomarr/api";
import { bootstrapSchema } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Loader2 } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { ErrorState } from "@/components/loomarr";
import { Button, Input, Label } from "@/components/ui";
import type { BootstrapStepProps } from "./bootstrap-step.type";

// Wizard step 1 — create the owning admin (§11, §13). Unauthenticated *because* it is
// gated on "no admin exists yet"; the first success closes the door (a second call 409s).
// Bootstrap issues no session, so on success we immediately sign the new admin in with
// the same credentials — the operator types their password once, not twice.
const BootstrapStep = ({ onDone }: BootstrapStepProps) => {
  const queryClient = useQueryClient();
  const [taken, setTaken] = useState(false);
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm({
    resolver: zodResolver(bootstrapSchema),
    defaultValues: { username: "", password: "", confirm: "" },
  });

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

  const busy = bootstrap.isPending || login.isPending;
  const blockingError = taken ? undefined : (bootstrap.error ?? login.error);

  if (taken) {
    return (
      <div className="flex flex-col gap-3">
        <p className="text-sm">This instance already has an owning admin — bootstrap only runs once.</p>
        <Link to="/login" className="w-fit text-sm text-tune underline-offset-4 hover:underline">
          Sign in instead
        </Link>
      </div>
    );
  }

  return (
    <form
      id="wizard-bootstrap"
      noValidate
      onSubmit={handleSubmit((v) =>
        bootstrap.mutate({ data: { username: v.username, password: v.password } }),
      )}
      className="flex flex-col gap-4"
    >
      {blockingError != null && <ErrorState error={blockingError} />}

      <div className="flex flex-col gap-2">
        <Label htmlFor="bootstrap-username">Username</Label>
        <Input id="bootstrap-username" autoComplete="username" {...register("username")} />
        {errors.username && (
          <p role="alert" className="text-onair-300 text-sm">
            {errors.username.message}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="bootstrap-password">Password</Label>
        <Input
          id="bootstrap-password"
          type="password"
          autoComplete="new-password"
          {...register("password")}
        />
        {errors.password && (
          <p role="alert" className="text-onair-300 text-sm">
            {errors.password.message}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="bootstrap-confirm">Confirm password</Label>
        <Input id="bootstrap-confirm" type="password" autoComplete="new-password" {...register("confirm")} />
        {errors.confirm && (
          <p role="alert" className="text-onair-300 text-sm">
            {errors.confirm.message}
          </p>
        )}
      </div>

      <Button type="submit" disabled={busy} className="w-fit">
        {busy && <Loader2 className="animate-spin" aria-hidden />}
        {busy ? "Creating…" : "Create admin"}
      </Button>
    </form>
  );
};

export { BootstrapStep };
