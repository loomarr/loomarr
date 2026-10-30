import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import type { RefObject } from "react";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { SourceClipSearch } from "../source-clip-search";
import { SourcePreviewSection } from "../source-preview-section";
import { SourceSettings } from "../source-settings";
import { SourceStatus } from "../source-status";
import type { SourceActions } from "../use-source-actions";

const sourceKindLabel = (source: FillerSourceDTO) => {
  switch (source.kind) {
    case "archive":
      return "Archive.org collection";
    case "youtube":
      return "YouTube channel or playlist";
    case "library":
      return "Media server library";
    default:
      return "Folder on this server";
  }
};

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
const SourceDetailSheet = ({ source, actions, finalFocus, onClose }: SourceDetailSheetProps) => (
  <Sheet
    open={Boolean(source)}
    onOpenChange={(open) => {
      if (!open) onClose();
    }}
    swipeDirection="right"
  >
    {source && (
      <SheetContent finalFocus={finalFocus}>
        <SheetHeader>
          <SheetTitle>{source.id === "folder" ? "Drop folder" : source.target}</SheetTitle>
          <SheetDescription>{sourceKindLabel(source)}</SheetDescription>
        </SheetHeader>

        <div key={source.id} className="flex flex-col gap-6 p-6">
          <SourceStatus
            source={source}
            fetching={actions.fetchSource.pending}
            fetchError={actions.fetchSource.error}
            onFetch={actions.fetchSource.run}
          />
          <SourcePreviewSection source={source} />
          {source.actions.includes("search") && <SourceClipSearch source={source} />}
          <SourceSettings source={source} actions={actions} />
        </div>
      </SheetContent>
    )}
  </Sheet>
);

export { SourceDetailSheet, sourceKindLabel };
