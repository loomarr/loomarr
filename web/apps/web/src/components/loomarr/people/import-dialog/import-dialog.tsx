import { Search, Server, ShieldCheck } from "lucide-react";
import { useId, useMemo, useState } from "react";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { ImportDialogProps } from "./import-dialog.type";

type CandidateFilter = "all" | "available" | "imported" | "disabled" | "admin";

const ImportDialog = ({
  available,
  candidates,
  candidateError,
  mutationError,
  importing = false,
  syncing = false,
  defaultOpen = false,
  portalContainer,
  onRetry,
  onImport,
  onSync,
}: ImportDialogProps) => {
  const searchId = useId();
  const filterId = useId();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<CandidateFilter>("all");
  const [picked, setPicked] = useState<Set<string>>(new Set());

  const visible = useMemo(() => {
    const term = query.trim().toLocaleLowerCase();
    return (candidates ?? []).filter((candidate) => {
      if (term && !candidate.name.toLocaleLowerCase().includes(term)) return false;
      if (filter === "available") return !candidate.imported;
      if (filter === "imported") return candidate.imported;
      if (filter === "disabled") return candidate.disabled;
      if (filter === "admin") return candidate.isAdmin;
      return true;
    });
  }, [candidates, filter, query]);

  const visibleImportable = visible.filter((candidate) => !candidate.imported);
  const selectedIds = [...picked].filter((id) =>
    candidates?.some((candidate) => candidate.id === id && !candidate.imported),
  );
  const allVisiblePicked =
    visibleImportable.length > 0 && visibleImportable.every((candidate) => picked.has(candidate.id));

  const toggle = (id: string) =>
    setPicked((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const toggleVisible = () =>
    setPicked((current) => {
      const next = new Set(current);
      for (const candidate of visibleImportable) {
        if (allVisiblePicked) next.delete(candidate.id);
        else next.add(candidate.id);
      }
      return next;
    });

  const importSelected = async () => {
    try {
      await onImport(selectedIds);
      setPicked(new Set());
    } catch {
      // The connected caller exposes the RFC 7807 problem through `error`; retaining the
      // selection makes a transient provider failure retryable without rebuilding the batch.
    }
  };

  const syncExisting = async () => {
    try {
      await onSync();
    } catch {
      // See importSelected: the problem stays visible in the focused workflow.
    }
  };

  return (
    <Dialog defaultOpen={defaultOpen}>
      <DialogTrigger render={<Button />}>
        <Server aria-hidden />
        Import from Emby/Jellyfin
      </DialogTrigger>
      <DialogContent
        portalContainer={portalContainer}
        className="max-h-[calc(100dvh-2rem)] max-w-3xl overflow-hidden p-0 sm:w-[calc(100%-2rem)]"
      >
        <DialogHeader className="border-static-800 border-b px-6 pt-6 pr-12 pb-4">
          <DialogTitle>Import from Emby/Jellyfin</DialogTitle>
          <DialogDescription>
            Choose which media-server accounts may sign in to Loomarr. Nothing is provisioned unless you
            explicitly select it.
          </DialogDescription>
        </DialogHeader>

        {!available ? (
          <div className="flex flex-col items-start gap-4 overflow-auto px-6 pb-6">
            <div className="rounded-lg border border-border bg-static-900 p-4">
              <p className="font-medium">Connect a media server first</p>
              <p className="mt-1 text-muted-foreground text-sm">
                Add Emby or Jellyfin under Connections. Once Loomarr can reach it, its accounts will be
                available here for explicit import.
              </p>
            </div>
            <Button render={<a href="/settings/connections" />} variant="outline">
              Open Connections settings
            </Button>
          </div>
        ) : (
          <>
            <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto px-6">
              <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_12rem]">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor={searchId}>Search accounts</Label>
                  <div className="relative">
                    <Search
                      className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground"
                      aria-hidden
                    />
                    <Input
                      id={searchId}
                      value={query}
                      onChange={(event) => setQuery(event.target.value)}
                      className="pl-9"
                      placeholder="Name"
                    />
                  </div>
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor={filterId}>Show</Label>
                  <Select value={filter} onValueChange={(value) => setFilter(value as CandidateFilter)}>
                    <SelectTrigger id={filterId}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All accounts</SelectItem>
                      <SelectItem value="available">Available to import</SelectItem>
                      <SelectItem value="imported">Already imported</SelectItem>
                      <SelectItem value="disabled">Disabled on server</SelectItem>
                      <SelectItem value="admin">Server admins</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              {mutationError != null && <ErrorState error={mutationError} className="p-4" />}

              {candidateError != null ? (
                <ErrorState error={candidateError} onRetry={onRetry} className="p-4" />
              ) : candidates === undefined ? (
                <div className="grid gap-2" role="status" aria-label="Loading media-server accounts">
                  {[0, 1, 2].map((item) => (
                    <div key={item} className="h-16 animate-pulse rounded-lg border border-border bg-card" />
                  ))}
                </div>
              ) : candidates.length === 0 ? (
                <EmptyState
                  title="No accounts reported"
                  description="Emby/Jellyfin did not return any accounts to import."
                />
              ) : visible.length === 0 ? (
                <div className="rounded-lg border border-border bg-card p-6 text-center">
                  <p className="font-medium">No matching accounts</p>
                  <p className="text-muted-foreground text-sm">Try another name or account filter.</p>
                </div>
              ) : (
                <div className="overflow-hidden rounded-lg border border-border bg-card">
                  <div className="flex flex-wrap items-center justify-between gap-2 border-static-800 border-b px-4 py-2">
                    <p aria-live="polite" className="text-muted-foreground text-sm">
                      {selectedIds.length} selected · {visibleImportable.length} importable shown
                    </p>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={visibleImportable.length === 0 || importing}
                      onClick={toggleVisible}
                    >
                      {allVisiblePicked ? "Clear visible" : "Select visible"}
                    </Button>
                  </div>
                  <ul>
                    {visible.map((candidate) => {
                      const id = `${searchId}-${candidate.id}`;
                      return (
                        <li key={candidate.id} className="border-static-800 border-b last:border-b-0">
                          <Label
                            htmlFor={id}
                            className="flex cursor-pointer items-start gap-3 px-4 py-3 font-normal has-[:disabled]:cursor-not-allowed"
                          >
                            <Checkbox
                              id={id}
                              checked={candidate.imported || picked.has(candidate.id)}
                              disabled={candidate.imported || importing}
                              onChange={() => toggle(candidate.id)}
                              className="mt-1"
                            />
                            <span className="min-w-0 flex-1">
                              <span className="flex flex-wrap items-center gap-2">
                                <span className="font-medium">{candidate.name}</span>
                                {candidate.imported && <Badge variant="lock">Already imported</Badge>}
                                {candidate.disabled && <Badge variant="onair">Disabled on server</Badge>}
                                {candidate.isAdmin && <Badge variant="tune">Server admin</Badge>}
                              </span>
                              <span className="mt-1 block text-static-400 text-xs">
                                Initial Loomarr role: {candidate.isAdmin ? "Admin" : "Member"}
                              </span>
                            </span>
                          </Label>
                        </li>
                      );
                    })}
                  </ul>
                </div>
              )}

              <div className="grid gap-3 rounded-lg border border-border bg-static-900 p-4 text-sm sm:grid-cols-2">
                <div>
                  <p className="flex items-center gap-2 font-medium">
                    <ShieldCheck className="size-4 text-lock" aria-hidden />
                    Passwords stay private
                  </p>
                  <p className="mt-1 text-muted-foreground">
                    Emby/Jellyfin validates the password. After a successful sign-in, Loomarr stores only a
                    non-reversible Argon2id verifier for outage access and refreshes it after later provider
                    sign-ins.
                  </p>
                </div>
                <div>
                  <p className="font-medium">Sync existing accounts</p>
                  <p className="mt-1 text-muted-foreground">
                    Refresh imported names and disabled state. Sync never provisions an unselected account.
                  </p>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="mt-3"
                    disabled={syncing || importing}
                    onClick={() => void syncExisting()}
                  >
                    {syncing ? "Syncing…" : "Sync existing"}
                  </Button>
                </div>
              </div>
            </div>

            <DialogFooter className="border-static-800 border-t px-6 py-4">
              <Button
                type="button"
                disabled={selectedIds.length === 0 || importing || syncing}
                onClick={() => void importSelected()}
              >
                {importing
                  ? "Importing…"
                  : `Import ${selectedIds.length} ${selectedIds.length === 1 ? "account" : "accounts"}`}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
};

export { ImportDialog };
