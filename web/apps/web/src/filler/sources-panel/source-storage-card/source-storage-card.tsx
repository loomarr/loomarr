import * as fillerApi from "@loomarr/api/endpoints/filler";
import { unwrap } from "@loomarr/api/unwrap";
import { formatBytes, pluralize } from "@loomarr/core/format";
import { Action, Text } from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useAuth } from "@/auth/use-auth";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { cn } from "@/lib/utils";

// The Storage card at the top of the Sources tab: how much the filler library uses, why new filler
// is paused (if it is), and — when the host drive's reserve is what paused it — the one-click
// cleanup of unfinished temporary downloads. Owns the readiness read and the cleanup mutation; when
// the read fails it renders that error in the card's place.
const SourceStorageCard = () => {
  const queryClient = useQueryClient();
  const { isAdmin } = useAuth();
  const readinessQuery = fillerApi.useFillerReadiness();
  const readiness = unwrap(readinessQuery.data, (body) => body);
  const storage = readiness?.storage;
  const cleanupPreviewQuery = fillerApi.usePreviewFillerStorageCleanup({
    query: { enabled: isAdmin && storage?.pausedBy === "host_reserve" },
  });
  const cleanupPreview = unwrap(cleanupPreviewQuery.data, (body) => body);
  const cleanupStorage = fillerApi.useCleanupFillerStorage({
    mutation: {
      onSuccess: async (response) => {
        const result = unwrap(response, (body) => body);
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: fillerApi.getFillerReadinessQueryKey() }),
          queryClient.invalidateQueries({ queryKey: fillerApi.getPreviewFillerStorageCleanupQueryKey() }),
        ]);
        if (!result) return;
        if (result.failedItems > 0) {
          toast.error("Some temporary files could not be removed", {
            description: `${pluralize(result.failedItems, "folder")} stayed in place. Loomarr will try them again later.`,
          });
          return;
        }
        toast.success(
          result.removedItems > 0 ? `Freed ${formatBytes(result.removedBytes)}` : "Nothing needed cleaning",
          { description: "Your filler library was not changed." },
        );
      },
    },
  });
  const cleanupResult = unwrap(cleanupStorage.data, (body) => body);

  if (!storage) {
    return readinessQuery.error ? (
      <ErrorState error={readinessQuery.error} onRetry={() => readinessQuery.refetch()} />
    ) : null;
  }

  // The server owns the warning threshold (storage state); a nearly full or full allowance is a
  // warning with the way out (#2737).
  const allowanceFull = storage.state === "approaching" || storage.pausedBy === "library_limit";
  return (
    <section
      aria-labelledby="filler-storage-heading"
      className={cn(
        "rounded-lg border p-4",
        allowanceFull ? "border-caution/40 bg-caution/5" : "border-border bg-card",
      )}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <h2 id="filler-storage-heading" className="text-sm font-medium">
            Storage
          </h2>
          <p className="mt-1 text-sm">
            {formatBytes(storage.managedBytes)} used for filler · {formatBytes(storage.freeBytes)} free on
            this drive
          </p>
          <Text as="p" style={{ marginTop: 4 }} textRole="hint" tone={allowanceFull ? "warning" : "muted"}>
            {storage.pausedBy === "host_reserve"
              ? `New filler is paused so this drive keeps ${formatBytes(storage.hardReserveBytes)} free.`
              : storage.pausedBy === "library_limit"
                ? `New filler is paused at its ${formatBytes(storage.softBudgetBytes)} allowance.`
                : storage.pausedBy === "capacity_unavailable"
                  ? "Loomarr cannot safely check the available space in this folder."
                  : storage.pausedBy === "estimate_unknown"
                    ? "A download could not be sized safely and was stopped. The folder is fine; Loomarr will try again."
                    : storage.state === "approaching"
                      ? `${formatBytes(storage.availableBytes)} is left for new filler. Loomarr will pause before it risks space kept for this drive.`
                      : `Automatic downloads will pause before this drive has less than ${formatBytes(storage.hardReserveBytes)} free.`}
            {storage.reservedBytes > 0
              ? ` ${formatBytes(storage.reservedBytes)} is set aside for work in progress.`
              : ""}
          </Text>
        </div>
        <Action
          accessibilityRole="link"
          render={
            <a
              href={`/filler/settings/${storage.pausedBy === "capacity_unavailable" ? "folders" : "storage"}`}
            />
          }
          variant={allowanceFull ? "outline" : "ghost"}
        >
          {storage.pausedBy === "capacity_unavailable"
            ? "Choose folder"
            : allowanceFull
              ? "Raise allowance"
              : "Storage options"}
        </Action>
      </div>

      {storage.pausedBy === "host_reserve" ? (
        <div className="border-border mt-3 border-t pt-3">
          {cleanupPreviewQuery.isLoading ? (
            <p aria-live="polite" className="text-muted-foreground text-sm">
              Checking for old temporary downloads…
            </p>
          ) : cleanupPreview && cleanupPreview.items > 0 ? (
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="text-muted-foreground text-sm">
                {formatBytes(cleanupPreview.bytes)} in{" "}
                {pluralize(cleanupPreview.items, "unfinished download")} is safe to remove. Clips in your
                library stay untouched.
              </p>
              <Action disabled={cleanupStorage.isPending} onPress={() => cleanupStorage.mutate()}>
                {cleanupStorage.isPending ? "Freeing space…" : `Free ${formatBytes(cleanupPreview.bytes)}`}
              </Action>
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">
              There are no old temporary downloads Loomarr can safely remove. Free space elsewhere on this
              drive or remove filler clips you no longer want.
            </p>
          )}
          {cleanupResult && cleanupResult.removedItems > 0 ? (
            <p role="status" className="text-muted-foreground mt-2 text-xs">
              Removed {pluralize(cleanupResult.removedItems, "temporary folder")} · freed{" "}
              {formatBytes(cleanupResult.removedBytes)}.
            </p>
          ) : null}
          {cleanupPreviewQuery.error || cleanupStorage.error ? (
            <p role="alert" className="text-destructive mt-2 text-sm">
              Temporary files could not be checked or removed. Nothing in your library was changed.
            </p>
          ) : null}
        </div>
      ) : null}
    </section>
  );
};

export { SourceStorageCard };
