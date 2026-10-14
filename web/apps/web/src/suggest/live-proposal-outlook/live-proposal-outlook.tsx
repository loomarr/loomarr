import { getProposalOutlook } from "@loomarr/api/endpoints/proposals";
import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { Proposal } from "@loomarr/api/models/proposal";
import { useQuery } from "@tanstack/react-query";
import { ProposalOutlook } from "@/components/loomarr/ai/proposal-outlook";

interface LiveProposalOutlookProps {
  id: string;
  proposal: Proposal;
  edit?: ApprovalEditDTO;
}

interface ProposalOutlookQuery {
  id?: string;
  proposal?: Proposal;
  edit?: ApprovalEditDTO;
}

const useProposalOutlook = ({ id, proposal, edit }: ProposalOutlookQuery) =>
  useQuery({
    queryKey: ["proposal-outlook", id ?? "", proposal ?? null, edit ?? {}],
    queryFn: async ({ signal }) => {
      if (!id) throw new Error("Proposal id is required");
      const result = await getProposalOutlook(id, edit, { signal });
      if (result.status !== 200) throw new Error("Outlook unavailable");
      return result.data;
    },
    enabled: id !== undefined && proposal !== undefined,
    retry: false,
    staleTime: 0,
  });

const LiveProposalOutlook = ({ id, proposal, edit }: LiveProposalOutlookProps) => {
  const query = useProposalOutlook({ id, proposal, edit });
  return (
    <ProposalOutlook
      pending={query.isFetching}
      assessment={query.isFetching || query.isError ? undefined : query.data}
    />
  );
};

export { LiveProposalOutlook, useProposalOutlook };
