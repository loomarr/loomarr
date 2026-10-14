import * as channelsApi from "@loomarr/api/endpoints/channels";
import * as imagesApi from "@loomarr/api/endpoints/images";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { PreviewChannelWatermarkHoliday } from "@loomarr/api/models/previewChannelWatermarkHoliday";
import type { PreviewChannelWatermarkPicture } from "@loomarr/api/models/previewChannelWatermarkPicture";
import type { WatermarkPolicy } from "@loomarr/api/models/watermarkPolicy";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { type OnScreenLogoPolicy, type OnScreenLogoPreviewRequest, OnScreenLogoSection } from "@loomarr/ui";
import { useQuery } from "@tanstack/react-query";
import { useRef } from "react";
import { toast } from "sonner";
import { useSettingsEntries } from "@/settings/use-settings-entries";

// The Channel info tab's On-screen logo section (#1659), wired to the backend: every choice is one
// PATCH of policy.watermark through the per-channel writer, like the Programming fields; the
// preview is the PNG the renderer would air (GET …/watermark/preview), and that response's
// drawable header drives the "can't draw" warning. The holidays (dates, in effect today, pictures),
// the era and the automatic name come from GET …/watermark/options: the page never re-derives them.

interface ChannelOnScreenLogoProps {
  channel: ChannelDTO;
  onOpenProgramming: () => void;
  savePolicy: (policy: ChannelPolicy) => void;
}

const ChannelOnScreenLogo = ({ channel, onOpenProgramming, savePolicy }: ChannelOnScreenLogoProps) => {
  const policy = channel.policy ?? {};
  const watermark: WatermarkPolicy = policy.watermark ?? {};
  const fileInput = useRef<HTMLInputElement>(null);

  const settings = useSettingsEntries();
  const setting = (key: string) => settings.find((s) => s.key === key)?.value;

  // A saved change (a new name, picture or image) changes the PNG at the same query, so the policy
  // itself versions the URL; it also keys the options, whose pictures follow the saved choice.
  const version = encodeURIComponent(JSON.stringify(watermark));
  const options = unwrap(
    channelsApi.useGetChannelWatermarkOptions(channel.id, {
      query: {
        queryKey: [
          ...channelsApi.getGetChannelWatermarkOptionsQueryKey(channel.id),
          version,
          policy.seasonal,
          policy.scope,
        ],
      },
    }).data,
  );
  const previewSrc = (r: OnScreenLogoPreviewRequest) => {
    const url = channelsApi.getPreviewChannelWatermarkUrl(channel.id, {
      color: r.color,
      holiday: (r.holiday ?? undefined) as PreviewChannelWatermarkHoliday | undefined,
      look: r.look,
      motifOnly: r.motifOnly || undefined,
      picture: r.picture as PreviewChannelWatermarkPicture | undefined,
      size: r.size,
    });
    return `${url}${url.includes("?") ? "&" : "?"}v=${version}`;
  };

  const drawable = useQuery({
    queryKey: ["watermark-drawable", channel.id, version],
    queryFn: async () => {
      const res = await fetch(previewSrc({ holiday: null }), { credentials: "include" });
      return res.headers.get("X-Loomarr-Watermark-Drawable") !== "false";
    },
  });

  const save = (next: WatermarkPolicy) => savePolicy({ ...policy, watermark: next });

  // The upload takes a while; edits made meanwhile are in the page's pending policy. Patch only
  // `image` onto the latest watermark, not the one captured when the upload began.
  const latest = useRef(watermark);
  latest.current = watermark;
  const latestPolicy = useRef(policy);
  latestPolicy.current = policy;
  const upload = async (file: File) => {
    try {
      const res = unwrap(await channelsApi.uploadChannelWatermark(channel.id, { file }));
      if (res?.image) {
        savePolicy({ ...latestPolicy.current, watermark: { ...latest.current, image: res.image } });
      }
    } catch (e) {
      toast.error(toProblem(e).title ?? "Couldn't upload the logo");
    }
  };

  // Until the options arrive the section would read "ignores holidays"; wait rather than mislead.
  if (!options) return null;

  return (
    <>
      <OnScreenLogoSection
        autoCallsign={options?.autoCallsign}
        channelNumber={channel.number}
        eraLabel={options?.era || undefined}
        everyHoliday={!policy.seasonal?.holidays?.length}
        defaults={{
          look: (setting("playout.watermark_look") ?? "text") as NonNullable<OnScreenLogoPolicy["look"]>,
          opacity: Number(setting("playout.watermark_opacity_pct") ?? 40) / 100,
        }}
        drawable={drawable.data ?? true}
        holidays={options?.holidays ?? []}
        // The stored upload, served by the image service at its largest icon rung (alpha kept in WebP).
        imageSrc={watermark.image ? imagesApi.getServeImageUrl(watermark.image, "w185.webp") : undefined}
        onChange={(patch) => save({ ...watermark, ...patch } as WatermarkPolicy)}
        onOpenProgramming={onOpenProgramming}
        onRemoveImage={() => save({ ...watermark, image: undefined })}
        onUploadImage={() => fileInput.current?.click()}
        policy={watermark as OnScreenLogoPolicy}
        previewSrc={previewSrc}
      />
      <input
        accept="image/png,image/webp"
        aria-hidden
        style={{ display: "none" }}
        onChange={(e) => {
          const file = e.currentTarget.files?.[0];
          if (file) void upload(file);
          e.currentTarget.value = "";
        }}
        ref={fileInput}
        tabIndex={-1}
        type="file"
      />
    </>
  );
};

export { ChannelOnScreenLogo };
