import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Stack,
} from "@loomarr/design-system";
import type { RefObject } from "react";
import { useFillerProviders } from "../../use-filler-providers";
import { SourceClipSearch } from "../source-clip-search";
import { SourcePreviewSection } from "../source-preview-section";
import { SourceSettings } from "../source-settings";
import { SourceStatus } from "../source-status";
import type { SourceActions } from "../use-source-actions";

interface SourceDetailSheetProps {
  /** The source whose drawer is open; the drawer is closed when there is none. */
  source?: FillerSourceDTO;
  actions: SourceActions;
  /** Where focus returns when the drawer closes: the control that opened it. */
  finalFocus: RefObject<HTMLElement | null>;
  onClose: () => void;
}

// The right-hand drawer for one registered source: its status, a few examples of what it holds, a
// clip search for Archive.org, and its settings. Each section owns its own draft state and is
// remounted per source (`key`), so opening a different source starts every section fresh.
const SourceDetailSheet = ({ source, actions, finalFocus, onClose }: SourceDetailSheetProps) => {
  const providers = useFillerProviders();
  return (
    <Dialog
      open={Boolean(source)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      {source && (
        <DialogContent finalFocus={finalFocus} placement="side">
          <DialogHeader>
            <DialogTitle>{source.id === "folder" ? "Drop folder" : source.target}</DialogTitle>
            <DialogDescription>{providers.find(source.kind)?.sourceNoun}</DialogDescription>
          </DialogHeader>

          <Stack className="p-6" gap="section" key={source.id}>
            <SourceStatus
              source={source}
              fetching={actions.fetchSource.pending}
              fetchError={actions.fetchSource.error}
              onFetch={actions.fetchSource.run}
            />
            <SourcePreviewSection source={source} />
            {source.actions.includes("search") && <SourceClipSearch source={source} />}
            <SourceSettings source={source} actions={actions} />
          </Stack>
        </DialogContent>
      )}
    </Dialog>
  );
};

export { SourceDetailSheet };
