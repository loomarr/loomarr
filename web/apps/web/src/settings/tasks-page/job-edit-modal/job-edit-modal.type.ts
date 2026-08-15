import type { JobView } from "@loomarr/api";

interface JobEditModalProps {
  job: JobView;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export type { JobEditModalProps };
