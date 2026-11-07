import type { FillerSourcePreviewItemDTO } from "@loomarr/api/models/fillerSourcePreviewItemDTO";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@loomarr/design-system";
import { ExternalLink } from "lucide-react";
import { Button } from "@/components/ui/button";

interface SourceItemPreviewDialogProps {
  /** The item to play; `embedUrl` is the provider's own player, absent when it cannot be embedded. */
  item?: Pick<FillerSourcePreviewItemDTO, "title" | "url" | "embedUrl">;
  /** The provider's operator-facing name, from the provider registry. */
  providerLabel: string;
  onClose: () => void;
}

const SourceItemPreviewDialog = ({ item, providerLabel, onClose }: SourceItemPreviewDialogProps) => {
  const embedURL = item?.embedUrl;
  return (
    <Dialog open={Boolean(item)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent size="reading">
        <DialogHeader>
          <DialogTitle>{item?.title ?? "Item preview"}</DialogTitle>
          <DialogDescription>
            This plays from {providerLabel}. Loomarr has not downloaded it.
          </DialogDescription>
        </DialogHeader>
        {embedURL ? (
          <iframe
            className="aspect-video w-full rounded-md border-0 bg-black"
            src={embedURL}
            title={item ? `Preview ${item.title}` : "Item preview"}
            allow="autoplay; encrypted-media; picture-in-picture"
            allowFullScreen
            referrerPolicy="strict-origin-when-cross-origin"
          />
        ) : (
          <div className="bg-muted text-muted-foreground flex aspect-video items-center justify-center rounded-md px-6 text-center text-sm">
            This provider cannot show the item here. Open the original to preview it.
          </div>
        )}
        <DialogFooter>
          {item && (
            <Button variant="outline" render={<a href={item.url} target="_blank" rel="noreferrer" />}>
              Open original
              <ExternalLink aria-hidden />
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export { SourceItemPreviewDialog };
