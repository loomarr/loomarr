import type { FillerSourcePreviewItemDTO } from "@loomarr/api/models/fillerSourcePreviewItemDTO";
import { ExternalLink } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface SourceItemPreviewDialogProps {
  item?: FillerSourcePreviewItemDTO;
  kind: "archive" | "youtube";
  onClose: () => void;
}

const providerEmbedURL = (kind: SourceItemPreviewDialogProps["kind"], raw: string) => {
  try {
    const source = new URL(raw);
    if (kind === "youtube") {
      const host = source.hostname.toLowerCase().replace(/^www\./, "");
      if (host !== "youtube.com") return;
      const id = source.searchParams.get("v") ?? "";
      if (!/^[a-z0-9_-]+$/i.test(id)) return;
      return `https://www.youtube-nocookie.com/embed/${encodeURIComponent(id)}?autoplay=1&playsinline=1`;
    }

    if (source.hostname.toLowerCase().replace(/^www\./, "") !== "archive.org") return;
    const match = source.pathname.match(/^\/details\/([^/]+)\/?$/);
    if (!match?.[1]) return;
    const id = decodeURIComponent(match[1]);
    if (!id || id.includes("/")) return;
    return `https://archive.org/embed/${encodeURIComponent(id)}?autoplay=1`;
  } catch {
    return;
  }
};

const SourceItemPreviewDialog = ({ item, kind, onClose }: SourceItemPreviewDialogProps) => {
  const embedURL = item ? providerEmbedURL(kind, item.url) : undefined;
  return (
    <Dialog open={Boolean(item)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="w-[calc(100%-2rem)] max-w-3xl p-4 sm:p-6">
        <DialogHeader className="pr-8">
          <DialogTitle>{item?.title ?? "Item preview"}</DialogTitle>
          <DialogDescription>
            This plays from {kind === "archive" ? "Archive.org" : "YouTube"}. Loomarr has not downloaded it.
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
          <div className="flex aspect-video items-center justify-center rounded-md bg-muted px-6 text-center text-muted-foreground text-sm">
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

export { providerEmbedURL, SourceItemPreviewDialog };
