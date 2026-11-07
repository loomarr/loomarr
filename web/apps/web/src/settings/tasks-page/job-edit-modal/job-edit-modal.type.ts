import type { JobView } from "@loomarr/api/models/jobView";
import type { SettingPreset } from "@loomarr/api/models/settingPreset";

interface JobEditModalProps {
  job: JobView;
  // The registry's presets on the job's schedule setting (job.scheduleKey).
  presets: readonly SettingPreset[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export type { JobEditModalProps };
