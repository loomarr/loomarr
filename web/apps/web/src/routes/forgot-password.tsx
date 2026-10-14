import * as authApi from "@loomarr/api/endpoints/auth";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { LoginShell } from "@/components/loomarr/setup/login-shell";
import { PasswordRecoveryRequest } from "@/components/loomarr/setup/password-recovery";
import { useDocumentTitle } from "@/lib/use-document-title";

const ForgotPasswordScreen = () => {
  useDocumentTitle("Forgot password");
  const [sent, setSent] = useState(false);
  const request = authApi.useRequestPasswordRecovery({
    mutation: { onSuccess: () => setSent(true) },
  });
  return (
    <LoginShell>
      <PasswordRecoveryRequest
        sent={sent}
        isPending={request.isPending}
        error={request.error}
        onSubmit={(username) => request.mutate({ data: { username } })}
      />
    </LoginShell>
  );
};

const Route = createFileRoute("/forgot-password")({ component: ForgotPasswordScreen });

export { Route };
