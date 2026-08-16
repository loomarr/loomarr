import { Link } from "@tanstack/react-router";
import { TriangleAlert } from "lucide-react";
import { useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import type { ProposalJobFailureProps } from "./proposal-job-failure.type";

const ProposalJobFailure = ({ failure, onRetry, onEdit }: ProposalJobFailureProps) => {
  const alertRef = useRef<HTMLDivElement>(null);
  useEffect(() => alertRef.current?.focus(), []);
  const canEdit = failure.code === "no_grounded_titles" && onEdit !== undefined;
  const settingsLink = failure.code === "timed_out" || failure.code === "provider_unavailable";

  return (
    <div
      ref={alertRef}
      role="alert"
      tabIndex={-1}
      className="flex flex-col gap-3 rounded-lg border border-onair-tint-15 bg-onair-tint-10 p-3 outline-none focus-visible:ring-2 focus-visible:ring-onair-300"
    >
      <div className="flex items-start gap-2 text-onair-300 text-sm">
        <TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
        <p>{failure.message}</p>
      </div>
      <div className="flex flex-wrap gap-2">
        {canEdit && (
          <Button size="sm" variant="outline" onClick={onEdit}>
            Edit description
          </Button>
        )}
        <Button size="sm" variant="suggest" onClick={onRetry}>
          Try again
        </Button>
        {settingsLink && (
          <Button size="sm" variant="ghost" render={<Link to="/settings/ai" />}>
            Open AI settings
          </Button>
        )}
      </div>
    </div>
  );
};

export { ProposalJobFailure };
