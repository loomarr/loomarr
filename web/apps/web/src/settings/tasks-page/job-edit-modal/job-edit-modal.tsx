import { jobsApi, settingsApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { ErrorState } from "@/components/loomarr";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui";
import { CRON_PRESETS, CUSTOM_VALUE, isPreset } from "../cron-presets";
import type { JobEditModalProps } from "./job-edit-modal.type";

// The "Modify Job" dialog (§18.1): edit a scheduled job's cron. Human-readable PRESETS are the
// default control (a dropdown); an **Advanced** toggle reveals the raw cron field for power
// users. Saving PATCHes the job's `scheduleKey` setting (an ordinary cron setting) — the BE
// validates the cron, so an invalid advanced expression comes back as a field error. The
// scheduler hot-applies the new cron on its next tick and emits a `job` SSE frame, so the
// Tasks list refreshes without a manual reload.
const JobEditModal = ({ job, open, onOpenChange }: JobEditModalProps) => {
  const queryClient = useQueryClient();
  // The picked preset value (a cron string) or CUSTOM_VALUE; seeded from the job's current
  // schedule — a matching preset if there is one, else Custom (advanced).
  const [choice, setChoice] = useState<string>(() => (isPreset(job.schedule) ? job.schedule : CUSTOM_VALUE));
  const [advancedCron, setAdvancedCron] = useState<string>(job.schedule);
  const advanced = choice === CUSTOM_VALUE;

  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async () => {
        await queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() });
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
          <div className="flex flex-col gap-1.5">
            <span className="font-medium text-sm">Frequency</span>
            <Select value={choice} onValueChange={setChoice}>
              <SelectTrigger aria-label="Frequency">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {CRON_PRESETS.map((p) => (
                  <SelectItem key={p.cron} value={p.cron}>
                    {p.label}
                  </SelectItem>
                ))}
                <SelectItem value={CUSTOM_VALUE}>Custom (advanced)…</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Advanced: the raw cron field, revealed only for the Custom choice. */}
          {advanced && (
            <div className="flex flex-col gap-1.5">
              <span className="font-medium text-sm">Cron expression</span>
              <Input
                aria-label="Cron expression"
                value={advancedCron}
                onChange={(e) => setAdvancedCron(e.target.value)}
                placeholder="0 */5 * * * *"
                spellCheck={false}
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
