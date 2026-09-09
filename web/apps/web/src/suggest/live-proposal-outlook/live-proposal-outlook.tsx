import { getProposalOutlook } from "@loomarr/api/endpoints/proposals";
import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { Proposal } from "@loomarr/api/models/proposal";
import { useQuery } from "@tanstack/react-query";
import { ProposalOutlook } from "@/components/loomarr/ai/proposal-outlook";

interface LiveProposalOutlookProps {
  id: string;
  onAddVariety?: () => void;
  proposal: Proposal;
  edit?: ApprovalEditDTO;
}

const LiveProposalOutlook = ({ id, proposal, edit, onAddVariety }: LiveProposalOutlookProps) => {
  const query = useQuery({
    queryKey: ["proposal-outlook", id, proposal, edit ?? {}],
    queryFn: async ({ signal }) => {
      const result = await getProposalOutlook(id, edit, { signal });
      if (result.status !== 200) throw new Error("Outlook unavailable");
      return result.data;
    },
    retry: false,
    staleTime: 0,
  });
  return (
    <ProposalOutlook
      onAddVariety={onAddVariety}
      pending={query.isFetching}
      assessment={query.isFetching || query.isError ? undefined : query.data}
    />
  );
};

export { LiveProposalOutlook };
