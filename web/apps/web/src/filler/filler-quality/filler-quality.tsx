import * as fillerApi from "@loomarr/api/endpoints/filler";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { DefectReview, StatePanel } from "@loomarr/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { ClipPreview } from "@/components/loomarr/filler/clip-preview";

// Filler › Manage › Quality (#2034 step 4c): the defect floor's household report as a labelling
// queue. The server pages the unlabelled clips worst first; the queue reads one at a time. A label
// takes its clip out of the list, so the same position then holds the next one; Skip moves past
// it, wrapping to the start once it runs off the end.
const FillerQuality = () => {
  const queryClient = useQueryClient();
  const [skipped, setSkipped] = useState(0);
  const query = fillerApi.useFillerDefectReport({ unlabelled: true, limit: 1, offset: skipped });
  const report = unwrap(query.data, (body) => body);
  const label = fillerApi.useLabelFillerDefect({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: fillerApi.getFillerDefectReportQueryKey() }),
    },
  });

  // Labelling the last clip in the list leaves the position past the end while skipped clips remain
  // before it: go back to the first of them rather than claim the queue is empty.
  const pastTheEnd = report !== undefined && report.rows.length === 0 && report.total > 0 && skipped > 0;
  useEffect(() => {
    if (pastTheEnd) setSkipped(0);
  }, [pastTheEnd]);

  if (query.error && !report) {
    return (
      <StatePanel
        action={{ label: "Try again", onPress: () => void query.refetch() }}
        description={toProblem(query.error).detail ?? "Loomarr couldn't read the defect report."}
        kind="error"
        title="Couldn't load the quality report"
      />
    );
  }
  if (!report || pastTheEnd) return <StatePanel kind="loading" title="Loading the quality report" />;

  const row = report.rows[0];
  const problem = label.error ? toProblem(label.error) : undefined;
  return (
    <DefectReview
      busy={label.isPending || query.isFetching}
      error={problem ? (problem.detail ?? problem.title) : undefined}
      onLabel={(clipId, value) => label.mutate({ clipId, data: { label: value } })}
      onSkip={() => setSkipped((n) => (n + 1 < report.total ? n + 1 : 0))}
      player={row ? <ClipPreview clip={{ hash: row.clipId, name: row.name }} /> : null}
      report={report}
    />
  );
};

export { FillerQuality };
