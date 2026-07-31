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

  const confirm = fillerApi.useConfirmFillerSplit({
    mutation: {
      onSuccess: (res) => {
        // Segments are clips now and the compilation row is gone — the catalog is the
        // surface that changed, so it refetches before the operator lands back on it.
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
  const p = unwrap(proposal.data);
  if (!p) return <p className="text-muted-foreground text-sm">Loading the proposal…</p>;

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="font-semibold text-xl">Review split</h1>
        <p className="mt-1 max-w-2xl text-muted-foreground text-sm">
          Detection proposed these cuts in <span className="font-mono text-static-200">{p.clipPath}</span>.
          Nothing is in the catalog yet. Edit, drop, or merge until the list is right, then confirm. Leaving
          keeps the proposal for later.
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
