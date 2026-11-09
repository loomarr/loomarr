import * as jobsApi from "@loomarr/api/endpoints/jobs";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import { invalidateSettings } from "@loomarr/core/events";
import {
  Action,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useAppForm } from "@/lib/app-form";
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

  // `choice` is the picked preset value (a cron string) or CUSTOM_VALUE, seeded from the job's
  // current schedule: a matching preset if there is one, else Custom (advanced).
  const form = useAppForm({
    defaultValues: {
      choice: isPreset(job.schedule, presets) ? job.schedule.trim() : CUSTOM_VALUE,
      advancedCron: job.schedule,
    },
    onSubmit: ({ value }) => {
      const cron = value.choice === CUSTOM_VALUE ? value.advancedCron.trim() : value.choice;
      patch.mutate({ data: { edits: { [job.scheduleKey]: cron } } });
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form
          className="flex flex-col gap-4"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            void form.handleSubmit();
          }}
        >
          <DialogHeader>
            <DialogTitle>Modify {job.title}</DialogTitle>
            <DialogDescription>How often this task runs.</DialogDescription>
          </DialogHeader>

          <form.AppField name="choice">
            {(field) => (
              <field.SelectField
                label="Frequency"
                options={[
                  ...presets.map((preset) => ({ label: preset.label, value: preset.value })),
                  { label: "Custom (advanced)…", value: CUSTOM_VALUE },
                ]}
              />
            )}
          </form.AppField>

          {/* Advanced: the raw cron field, revealed only for the Custom choice. */}
          <form.Subscribe selector={(state) => state.values.choice === CUSTOM_VALUE}>
            {(advanced) =>
              advanced && (
                <div className="flex flex-col gap-1.5">
                  <form.AppField name="advancedCron">
                    {(field) => (
                      <field.TextField
                        className="gap-1.5"
                        label="Cron expression"
                        placeholder="0 */5 * * * *"
                      />
                    )}
                  </form.AppField>
                  <p className="text-muted-foreground text-xs">
                    6-field, seconds first (e.g. <code>0 */5 * * * *</code> = every 5 minutes).
                  </p>
                </div>
              )
            }
          </form.Subscribe>

          {patch.error != null && <ErrorState error={patch.error} />}

          <DialogFooter>
            <Action disabled={patch.isPending} onPress={() => onOpenChange(false)} variant="ghost">
              Cancel
            </Action>
            <Action
              // A real submit button under Action's look, so Enter in the cron field still submits.
              render={<button disabled={patch.isPending} type="submit" />}
            >
              {patch.isPending ? "Saving…" : "Save"}
            </Action>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
};

export { JobEditModal };
