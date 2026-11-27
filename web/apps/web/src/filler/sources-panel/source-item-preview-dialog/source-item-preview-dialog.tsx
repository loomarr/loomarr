import type { FillerSourcePreviewItemDTO } from "@loomarr/api/models/fillerSourcePreviewItemDTO";
import {
  Action,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Stack,
  Text,
} from "@loomarr/design-system";

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
            style={{
              aspectRatio: "16 / 9",
              backgroundColor: "black",
              border: 0,
              borderRadius: 6,
              width: "100%",
            }}
            src={embedURL}
            title={item ? `Preview ${item.title}` : "Item preview"}
            allow="autoplay; encrypted-media; picture-in-picture"
            allowFullScreen
            referrerPolicy="strict-origin-when-cross-origin"
          />
        ) : (
          <Stack
            align="center"
            aspectRatio={16 / 9}
            backgroundColor="surfaceRaised"
            borderRadius={6}
            direction="row"
            justify="center"
            paddingHorizontal={24}
          >
            <Text as="p" textAlign="center" textRole="compact" tone="secondary">
              This provider cannot show the item here. Open the original to preview it.
            </Text>
          </Stack>
        )}
        <DialogFooter>
          {item && (
            <Action
              accessibilityRole="link"
              icon="external"
              render={<a href={item.url} target="_blank" rel="noreferrer" />}
              variant="outline"
            >
              Open original
            </Action>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export { SourceItemPreviewDialog };
