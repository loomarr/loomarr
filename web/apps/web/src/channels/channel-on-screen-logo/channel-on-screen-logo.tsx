import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { PreviewChannelWatermarkHoliday } from "@loomarr/api/models/previewChannelWatermarkHoliday";
import type { WatermarkPolicy } from "@loomarr/api/models/watermarkPolicy";
import { unwrap } from "@loomarr/api/unwrap";
import { type OnScreenLogoPolicy, type OnScreenLogoPreviewRequest, OnScreenLogoSection } from "@loomarr/ui";
import { useQuery } from "@tanstack/react-query";
import { useRef } from "react";
import { useSettingsEntries } from "@/settings/use-settings-entries";

// The Channel info tab's On-screen logo section (#1659), wired to the backend: every choice is one
// PATCH of policy.watermark through the per-channel writer, like the Programming fields; the
// preview is the PNG the renderer would air (GET …/watermark/preview), and that response's
// drawable header drives the "can't draw" warning. Holiday names come from the BE vocabulary.

// The rule vocabulary publishes each built-in holiday as a `holiday:<id>` when-token.
const HOLIDAY_PREFIX = "holiday:";

interface ChannelOnScreenLogoProps {
  channel: ChannelDTO;
  onOpenProgramming: () => void;
  savePolicy: (policy: ChannelPolicy) => void;
}

const ChannelOnScreenLogo = ({ channel, onOpenProgramming, savePolicy }: ChannelOnScreenLogoProps) => {
  const policy = channel.policy ?? {};
  const watermark: WatermarkPolicy = policy.watermark ?? {};
  const fileInput = useRef<HTMLInputElement>(null);

  const vocabulary = unwrap(
    channelsApi.useGetProgrammingVocabulary({ query: { staleTime: Number.POSITIVE_INFINITY } }).data,
  );
  const settings = useSettingsEntries();
  const setting = (key: string) => settings.find((s) => s.key === key)?.value;

  const seasonal = policy.seasonal;
  const builtIn = (vocabulary?.when ?? [])
    .filter((w) => w.token.startsWith(HOLIDAY_PREFIX))
    .map((w) => ({ id: w.token.slice(HOLIDAY_PREFIX.length), name: w.label }));
  const chosen = seasonal?.holidays ?? [];
  const holidays =
    seasonal?.mode === "off" ? [] : chosen.length ? builtIn.filter((h) => chosen.includes(h.id)) : builtIn;

  // A saved change (a new name or image) changes the PNG at the same query, so the policy
  // itself versions the URL.
  const version = encodeURIComponent(JSON.stringify(watermark));
  const previewSrc = (r: OnScreenLogoPreviewRequest) =>
    `${channelsApi.getPreviewChannelWatermarkUrl(channel.id, {
      color: r.color,
      holiday: (r.holiday ?? undefined) as PreviewChannelWatermarkHoliday | undefined,
      look: r.look,
      size: r.size,
    })}${r.look || r.color || r.holiday || r.size ? "&" : "?"}v=${version}`;

  const drawable = useQuery({
    queryKey: ["watermark-drawable", channel.id, version],
    queryFn: async () => {
      const res = await fetch(previewSrc({ holiday: null }), { credentials: "include" });
      return res.headers.get("X-Loomarr-Watermark-Drawable") !== "false";
    },
  });

  const save = (next: WatermarkPolicy) => savePolicy({ ...policy, watermark: next });

  const upload = async (file: File) => {
    const res = unwrap(await channelsApi.uploadChannelWatermark(channel.id, { file }));
    if (res?.image) save({ ...watermark, image: res.image });
  };

  return (
    <>
      <OnScreenLogoSection
        channelNumber={channel.number}
        defaults={{
          look: (setting("playout.watermark_look") ?? "text") as NonNullable<OnScreenLogoPolicy["look"]>,
          opacity: Number(setting("playout.watermark_opacity_pct") ?? 40) / 100,
        }}
        drawable={drawable.data ?? true}
        holidays={holidays}
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
