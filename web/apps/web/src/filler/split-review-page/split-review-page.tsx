import { fillerApi, isOk, toProblem, unwrap } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import { useAuth } from "@/auth";
import { EmptyState, ErrorState, SplitReviewEditor } from "@/components/loomarr";
import { useDocumentTitle } from "@/lib";
import type { SplitReviewPageProps } from "./split-review-page.type";

// SplitReviewPage — the /filler/splits/$proposalId screen (§10 V34). Detection persisted
// a proposal; this page reads it back (the GET is the truth — the SSE frame was only the
// doorbell) and hands it to the editor. Confirm commits the operator's edited cut list;
// Back leaves the proposal persisted for later — there is deliberately no reject endpoint.
const SplitReviewPage = ({ proposalId }: SplitReviewPageProps) => {
  useDocumentTitle("Review split");
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { isAdmin, isLoading: authLoading } = useAuth();

  // Admin-gated as a courtesy — every split route 403s for a member server-side anyway
  // (§11, §19); the gate turns a page of failed requests into an explanation.
  const proposal = fillerApi.useGetFillerSplit(proposalId, { query: { enabled: isAdmin } });
  const persistedProposal = unwrap(proposal.data);
  // Resolve the composite through the existing exact-hash catalog read. Composites are deliberately
  // absent from the airable catalog by default, so omitting includeComposites would turn a valid
  // parent into a miss and tempt this page back to exposing its storage hash.
  const parent = fillerApi.useListFiller(
    {
      hashes: persistedProposal ? [persistedProposal.clipHash] : [],
      includeComposites: true,
      limit: 1,
    },
    { query: { enabled: isAdmin && Boolean(persistedProposal) } },
  );

  const confirm = fillerApi.useConfirmFillerSplit({
    mutation: {
      onSuccess: (res) => {
        // Segments are clips now and the compilation remains as their non-airable parent. The
        // catalog is the surface that changed, so it refetches before the operator lands back on it.
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerQueryKey() });
        const clips = isOk(res) ? res.data.clips : 0;
        toast.success(clips > 0 ? `Split into ${clips} clips` : "Split confirmed");
        void navigate({ to: "/filler" });
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't confirm the split"),
    },
  });

  if (authLoading) return null;
  if (!isAdmin) {
    return (
      <EmptyState
        title="Admins only"
        description="Reviewing compilation splits decides what enters the catalog, so it's an admin action. Ask an admin to review this proposal."
      />
    );
  }

  if (proposal.error != null) {
    return <ErrorState error={proposal.error} onRetry={() => proposal.refetch()} />;
  }
  const p = persistedProposal;
  if (!p) return <p className="text-muted-foreground text-sm">Loading the proposal…</p>;
  const parentName = unwrap(parent.data, (body) => body.clips[0]?.name) || "this compilation";

  return (
    // p-6 for the same reason as the catalog page: the shell adds no gutter, so a page
    // without one renders flush against the sidebar.
    <div className="flex flex-col gap-4 p-6">
      <div>
        <h1 className="font-semibold text-xl">Review split</h1>
        <p className="mt-1 max-w-2xl text-muted-foreground text-sm">
          Detection proposed these cuts in <span className="font-medium text-static-200">{parentName}</span>.
          Confirming files the segments under the original compilation. Edit, drop, or merge until the list is
          right; leaving keeps the proposal for later.
        </p>
      </div>
      <SplitReviewEditor
        proposal={p}
        confirming={confirm.isPending}
        onConfirm={(segments) => confirm.mutate({ proposalId, data: { segments } })}
        onBack={() => void navigate({ to: "/filler" })}
      />
    </div>
  );
};

export { SplitReviewPage };
