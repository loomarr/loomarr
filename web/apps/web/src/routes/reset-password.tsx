import * as authApi from "@loomarr/api/endpoints/auth";
import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { clearAccountActionGrant, takeAccountActionGrantFromLocation } from "@/auth/account-action-grant";
import { LoginShell } from "@/components/loomarr/setup/login-shell";
import { PasswordRecoveryReset } from "@/components/loomarr/setup/password-recovery";
import { useDocumentTitle } from "@/lib/use-document-title";

const ResetPasswordScreen = () => {
  useDocumentTitle("Reset password");
  const { grant } = Route.useRouteContext();
  const started = useRef(false);
  const [succeeded, setSucceeded] = useState(false);
  const preview = authApi.usePreviewPasswordRecovery();
  const redeem = authApi.useRedeemPasswordRecovery({
    mutation: {
      onSuccess: () => {
        clearAccountActionGrant();
        setSucceeded(true);
      },
    },
  });

  useEffect(() => {
    if (!grant || started.current) return;
    started.current = true;
    preview.mutate({ data: { grant } });
  }, [grant, preview.mutate]);
  useEffect(() => () => clearAccountActionGrant(), []);

  return (
    <LoginShell className="py-10">
      <PasswordRecoveryReset
        expiresAt={preview.data?.status === 200 ? preview.data.data.expiresAt : undefined}
        isLoading={Boolean(grant) && preview.isPending}
        isRedeeming={redeem.isPending}
        succeeded={succeeded}
        error={redeem.error ?? preview.error}
        onRedeem={(password) => {
          if (!grant) return;
          redeem.mutate({ data: { grant, password } });
        }}
      />
    </LoginShell>
  );
};

const Route = createFileRoute("/reset-password")({
  beforeLoad: () => ({ grant: takeAccountActionGrantFromLocation() }),
  component: ResetPasswordScreen,
});

export { Route };
