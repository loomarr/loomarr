import * as fillerApi from "@loomarr/api/endpoints/filler";
import { unwrap } from "@loomarr/api/unwrap";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Text,
} from "@loomarr/design-system";
import { type RefObject, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ClipDetails } from "../../clip-details";
import { ClipDetailsEditor } from "../../clip-details-editor";
import { useFillerInvalidate } from "../../use-filler-invalidate";
import { ClipMoreDetails } from "../clip-more-details";

interface ClipInspectorBodyProps {
  hash: string;
  isAdmin: boolean;
}

// One clip's panel: read-only details first, with an admin's optional editor in its place. The
// clip is resolved by exact hash rather than from the current result page, so a segment expanded
// inside a recording opens too. Remount per hash (`key`) so a different clip never opens mid-edit.
const ClipInspectorBody = ({ hash, isAdmin }: ClipInspectorBodyProps) => {
  const [editing, setEditing] = useState(false);
  const { invalidateLifecycle } = useFillerInvalidate();
  const inspection = fillerApi.useListFiller({
    hashes: [hash],
    includeHeld: true,
    includeComposites: true,
    limit: 1,
  });
  const clip = unwrap(inspection.data, (body) => body.clips.find((candidate) => candidate.hash === hash));

  return (
    <>
      <DialogHeader>
        <DialogTitle>{clip?.name ?? "Clip details"}</DialogTitle>
        <DialogDescription>
          {editing ? "Edit details" : clip?.held ? "Preparing" : clip?.isComposite ? "Recording" : "Ready"}
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-5 p-6">
        {inspection.error ? (
          <ErrorState error={inspection.error} onRetry={() => inspection.refetch()} />
        ) : clip ? (
          editing && isAdmin ? (
            <ClipDetailsEditor
              key={clip.hash}
              clip={clip}
              onClose={() => setEditing(false)}
              onSaved={() => {
                setEditing(false);
                invalidateLifecycle();
              }}
            />
          ) : (
            <>
              <ClipDetails clip={clip} {...(isAdmin ? { onEdit: () => setEditing(true) } : {})} />
              <ClipMoreDetails clip={clip} />
            </>
          )
        ) : (
          <div role="status">
            <Text as="p" textRole="compact" tone="secondary">
              {inspection.isLoading ? "Loading clip…" : "This clip is no longer available."}
            </Text>
          </div>
        )}
      </div>
    </>
  );
};

interface ClipInspectorSheetProps {
  /** The clip being inspected; the sheet is closed when there is none. */
  hash?: string;
  isAdmin: boolean;
  /** Where focus returns when the sheet closes: the control that opened it. */
  finalFocus: RefObject<HTMLElement | null>;
  onClose: () => void;
}

// The right-hand panel that opens from a catalog card or row.
const ClipInspectorSheet = ({ hash, isAdmin, finalFocus, onClose }: ClipInspectorSheetProps) => (
  <Dialog
    open={Boolean(hash)}
    onOpenChange={(open) => {
      if (!open) onClose();
    }}
  >
    {hash ? (
      <DialogContent finalFocus={finalFocus} placement="side">
        <ClipInspectorBody key={hash} hash={hash} isAdmin={isAdmin} />
      </DialogContent>
    ) : null}
  </Dialog>
);

export { ClipInspectorSheet };
