import { suggestionsApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useAuth } from "@/auth";
import { ErrorState, GenerationProgress, ProposalReview } from "@/components/loomarr";
import { Button } from "@/components/ui";
import { useDocumentTitle } from "@/lib";
import { ApprovalQueue, IntentForm, useSuggestionRun } from "@/suggest";

// The Suggest workspace (§8, §12) — the product's core loop: a sentence becomes a
// grounded lineup. The wizard's guided first channel hands off here with ?intent=
// prefilled (§13), which is why the search param is part of the route contract.
interface SuggestSearch {
  intent?: string;
}

const SuggestScreen = () => {
  useDocumentTitle("Suggest");
  const { isAdmin } = useAuth();
  const queryClient = useQueryClient();
  const { intent } = Route.useSearch();
  const run = useSuggestionRun();

  const approve = suggestionsApi.useApproveProposal({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: suggestionsApi.getListProposalsQueryKey() }),
    },
  });
  const deny = suggestionsApi.useDenyProposal({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: suggestionsApi.getListProposalsQueryKey() }),
    },
  });

  const proposal = run.proposal;

  return (
    <div className="flex h-full flex-col">
      <header className="border-border border-b px-6 py-4">
        <h1 className="font-semibold text-xl">Suggest</h1>
        <p className="mt-1 text-muted-foreground text-sm">
          Describe a channel. Loomarr grounds every pick against your library and TMDB — it never invents a
          title.
        </p>
      </header>

      <div className="flex flex-1 flex-col gap-6 overflow-auto p-6">
        {!run.isRunning && !proposal && (
          <IntentForm initialDescription={intent} onSubmit={run.start} submitting={run.isRunning} />
        )}

        {run.error != null && <ErrorState error={run.error} />}

        {run.isRunning && <GenerationProgress phase={run.phase ?? "searching"} />}

        {proposal && (
          <div className="flex flex-col gap-4">
            <ProposalReview
              proposal={proposal.proposal}
              status={proposal.status}
              busy={approve.isPending || deny.isPending}
              // Approval is admin-only (§7, §11) — a member sees the review without the
              // controls, and the queue below is where an admin acts on others' work.
              onApprove={isAdmin ? () => approve.mutate({ id: proposal.id }) : undefined}
              onDeny={isAdmin ? () => deny.mutate({ id: proposal.id, data: {} }) : undefined}
            />
            {(approve.error ?? deny.error) != null && <ErrorState error={approve.error ?? deny.error} />}
            <Button variant="outline" className="w-fit" onClick={run.reset}>
              Start another
            </Button>
          </div>
        )}

        {/* The gate itself (§7): an admin sees everything awaiting approval, including
            other people's, since approving is the only path to spending real resources. */}
        {isAdmin && (
          <section className="flex flex-col gap-3 border-border border-t pt-6">
            <h2 className="font-semibold text-lg">Awaiting approval</h2>
            <ApprovalQueue />
          </section>
        )}
      </div>
    </div>
  );
};

const Route = createFileRoute("/_authed/suggest")({
  validateSearch: (search: Record<string, unknown>): SuggestSearch => ({
    intent: typeof search.intent === "string" ? search.intent : undefined,
  }),
  component: SuggestScreen,
});

export { Route };
