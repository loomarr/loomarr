import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatClipDuration, pluralize } from "@loomarr/core/format";
import { type ReactNode, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Disclosure } from "@/components/ui/disclosure";

interface CompositeCatalogGroupProps {
  clip: ClipDTO;
  onManage: () => void;
  renderParent: (clip: ClipDTO) => ReactNode;
  renderChild: (clip: ClipDTO) => ReactNode;
}

// A compilation in the catalog: the ordinary clip surface as the parent, plus a disclosure that
// loads its filed segments only once opened.
const CompositeCatalogGroup = ({ clip, onManage, renderParent, renderChild }: CompositeCatalogGroupProps) => {
  const [open, setOpen] = useState(false);
  const children = fillerApi.useListFiller(
    { parentHash: clip.hash, limit: 500 },
    { query: { enabled: open } },
  );
  const rows = unwrap(children.data, (body) => body.clips) ?? [];
  const total = unwrap(children.data, (body) => body.total) ?? 0;

  return (
    <div className="flex flex-col gap-2">
      {/* Keep the ordinary clip surface as the parent. A compilation still needs the same
          inspection entry point and confirmed split action; replacing it with a
          bespoke group heading made those established actions disappear. The disclosure below
          adds hierarchy without creating a second, weaker representation of the parent. */}
      {renderParent(clip)}
      <Disclosure open={open} onOpenChange={setOpen}>
        <Card className="overflow-hidden">
          <div className="flex flex-wrap items-center gap-3 p-3">
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">Compilation segments</p>
              <p className="text-muted-foreground font-mono text-xs">
                {formatClipDuration(clip.durationMs)} source reel
              </p>
            </div>
            {open && children.isFetching ? (
              <span className="text-muted-foreground text-xs">Loading segments…</span>
            ) : null}
            {open && !children.isFetching ? (
              <Button variant="outline" size="sm" onClick={onManage}>
                Manage {pluralize(total, "segment")}
              </Button>
            ) : null}
            <Disclosure.Trigger label={`${open ? "Hide" : "Show"} segments from ${clip.name}`} />
          </div>
          <Disclosure.Panel className="border-border border-t p-3">
            {children.error ? (
              <p className="text-onair-300 text-sm">Segments could not be loaded.</p>
            ) : rows.length > 0 ? (
              <div className="border-border overflow-hidden rounded-lg border">{rows.map(renderChild)}</div>
            ) : children.isFetching ? null : (
              <p className="text-muted-foreground text-sm">
                No filed segments remain under this compilation.
              </p>
            )}
          </Disclosure.Panel>
        </Card>
      </Disclosure>
    </div>
  );
};

export type { CompositeCatalogGroupProps };
export { CompositeCatalogGroup };
