import * as authApi from "@loomarr/api/endpoints/auth";
import * as invitationApi from "@loomarr/api/endpoints/invitations";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { clearInvitationGrant, takeInvitationGrantFromLocation } from "@/auth/invitation-grant";
import { InvitationJoin } from "@/components/loomarr/people/invitation-join";
import { LoginShell } from "@/components/loomarr/setup/login-shell";
import { useDocumentTitle } from "@/lib/use-document-title";

const JoinScreen = () => {
  useDocumentTitle("Join");
  const { grant } = Route.useRouteContext();
  const started = useRef(false);
  const router = useRouter();
  const queryClient = useQueryClient();
  const preview = invitationApi.usePreviewInvitation();
  const redeem = invitationApi.useRedeemInvitation({
    mutation: {
      onSuccess: async () => {
        clearInvitationGrant();
        await queryClient.invalidateQueries({ queryKey: authApi.getMeQueryKey() });
        router.history.replace("/guide");
      },
    },
  });

  useEffect(() => {
    if (!grant || started.current) return;
    started.current = true;
    preview.mutate({ data: { grant } });
  }, [grant, preview.mutate]);

  useEffect(() => () => clearInvitationGrant(), []);

  return (
    <LoginShell className="py-10">
      <InvitationJoin
        preview={preview.data?.status === 200 ? preview.data.data : undefined}
        isLoading={Boolean(grant) && preview.isPending}
        isRedeeming={redeem.isPending}
        error={redeem.error ?? preview.error}
        onRedeem={(values) => {
          if (!grant) return;
          redeem.mutate({ data: { grant, ...values } });
        }}
      />
    </LoginShell>
  );
};

const Route = createFileRoute("/join")({
  // This runs before JoinScreen renders, so the fragment is gone from browser
  // history before password controls enter the DOM. The returned value lives only
  // in the route's in-memory context and is cleared when the route unmounts.
  beforeLoad: () => ({ grant: takeInvitationGrantFromLocation() }),
  component: JoinScreen,
});

export { Route };
