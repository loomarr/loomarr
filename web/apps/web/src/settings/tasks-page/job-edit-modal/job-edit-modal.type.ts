import type { JobView } from "@loomarr/api/models/jobView";

interface JobEditModalProps {
  job: JobView;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export type { JobEditModalProps };
