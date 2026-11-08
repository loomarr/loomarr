import * as jobsApi from "@loomarr/api/endpoints/jobs";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import { invalidateSettings } from "@loomarr/core/events";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Field,
} from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { BrowserSelectControl } from "@/client-adapters";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { CUSTOM_VALUE, isPreset } from "../cron-presets";
import type { JobEditModalProps } from "./job-edit-modal.type";

// The "Modify Job" dialog (§18.1): edit a scheduled job's cron. Human-readable PRESETS are the
// default control (a dropdown); an **Advanced** toggle reveals the raw cron field for power
// users. Saving PATCHes the job's `scheduleKey` setting (an ordinary cron setting) — the BE
// validates the cron, so an invalid advanced expression comes back as a field error. The
// scheduler hot-applies the new cron on its next tick and emits a `job` SSE frame, so the
// Tasks list refreshes without a manual reload.
const JobEditModal = ({ job, presets, open, onOpenChange }: JobEditModalProps) => {
  const queryClient = useQueryClient();
  // The picked preset value (a cron string) or CUSTOM_VALUE; seeded from the job's current
  // schedule — a matching preset if there is one, else Custom (advanced).
  const [choice, setChoice] = useState<string>(() =>
    isPreset(job.schedule, presets) ? job.schedule.trim() : CUSTOM_VALUE,
  );
  const [advancedCron, setAdvancedCron] = useState<string>(job.schedule);
  const advanced = choice === CUSTOM_VALUE;

  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async () => {
        await invalidateSettings(queryClient);
        // The scheduler re-reads the cron on its next tick; nudge /v1/jobs so the new
        // schedule + next-run show immediately rather than on the next SSE frame. Use the
        // generated query key (["/v1/jobs"]) — a hand-written ["jobsList"] matches nothing.
        await queryClient.invalidateQueries({ queryKey: jobsApi.getJobsListQueryKey() });
        toast.success(`Updated ${job.title} schedule`);
        onOpenChange(false);
      },
    },
  });

  const save = () => {
    const cron = advanced ? advancedCron.trim() : choice;
    patch.mutate({ data: { edits: { [job.scheduleKey]: cron } } });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Modify {job.title}</DialogTitle>
          <DialogDescription>How often this task runs.</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          <BrowserSelectControl
            appearance="field"
            label="Frequency"
            onValueChange={setChoice}
            options={[
              ...presets.map((preset) => ({ label: preset.label, value: preset.value })),
              { label: "Custom (advanced)…", value: CUSTOM_VALUE },
            ]}
            value={choice}
          />

          {/* Advanced: the raw cron field, revealed only for the Custom choice. */}
          {advanced && (
            <div className="flex flex-col gap-1.5">
              <Field
                autoCapitalize="none"
                autoCorrect={false}
                label="Cron expression"
                onChangeText={setAdvancedCron}
                placeholder="0 */5 * * * *"
                spellCheck={false}
                value={advancedCron}
              />
              <p className="text-muted-foreground text-xs">
                6-field, seconds first (e.g. <code>0 */5 * * * *</code> = every 5 minutes).
              </p>
            </div>
          )}

          {patch.error != null && <ErrorState error={patch.error} />}
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={patch.isPending}>
            Cancel
          </Button>
          <Button onClick={save} disabled={patch.isPending}>
            {patch.isPending ? "Saving…" : "Save"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export { JobEditModal };
