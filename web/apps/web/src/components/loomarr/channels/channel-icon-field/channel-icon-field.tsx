import * as channelsApi from "@loomarr/api/endpoints/channels";
import { ApiError, toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { ImageOff, Loader2, Upload } from "lucide-react";
import { useId, useRef, useState } from "react";
import { CollapsibleSection } from "@/components/loomarr/feedback/collapsible-section";
import { Button } from "@/components/ui/button";
import { Image } from "@/components/ui/image";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import type { ChannelIconFieldProps } from "./channel-icon-field.type";

// ⚠ The multipart upload used to be hand-written here, replicating the mutator's transport
// contract (same-origin, `credentials: "include"`, the double-submit CSRF header) because
// POST …/icon was a raw mux handler outside the generated client. It is a Huma operation with
// a huma.MultipartFormFiles body now, so orval generates the call — it builds the FormData
// itself, and customFetch adds CSRF and credentials without touching the body.
//
// The old comment justified the copy by saying "customFetch always JSON-encodes the body".
// That was not true even then: the mutator passes `options.body` straight through to fetch.
const MAX_UPLOAD_BYTES = 2 * 1024 * 1024;

// ChannelIconField — the channel-detail "info" section's icon picker. One current-icon
// preview, three ways in (pick from the lineup's own TMDB posters, upload a file, paste a
// URL), and a clear. All three ways converge on the same channel value: `logo`. The TMDB
// suggestions and the direct-URL paste both just PATCH `logo` through the channel's own
// update mutation — no bespoke endpoint on that path. Upload is the one exception (it has
// to ship bytes), and even it ends by handing the resulting URL back to the channel record
// server-side, so from here it's still "the icon changed, refetch."
//
// A non-admin sees the current icon (or the placeholder) and nothing else — the editing
// affordances are gated on `isAdmin`, same as the rest of the admin-only editing surface
// on this page (§7 authorization model).
const ChannelIconField = ({
  channelId,
  logo,
  logoImage,
  onSetLogo,
  isAdmin = false,
  className,
}: ChannelIconFieldProps) => {
  const urlInputId = useId();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [urlDraft, setUrlDraft] = useState("");
  const [savingUrl, setSavingUrl] = useState(false);
  const [urlError, setUrlError] = useState<string>();

  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string>();
  const upload = channelsApi.useUploadChannelIcon();

  const [clearing, setClearing] = useState(false);

  // Suggestions load lazily — only once an admin actually opens the section — rather than
  // firing on every channel-page visit for a section most edits never touch.
  const [suggestionsOpen, setSuggestionsOpen] = useState(false);
  const suggestions = channelsApi.useChannelIconSuggestions(channelId, {
    query: { enabled: isAdmin && suggestionsOpen },
  });
  const suggestionList = unwrap(suggestions.data, (b) => b.suggestions) ?? [];
  // A 501 means TMDB isn't configured on this install — that's a normal, expected shape
  // (not every deployment grounds against TMDB), so it renders as guidance, not an error.
  const tmdbUnconfigured = suggestions.error instanceof ApiError && suggestions.error.status === 501;

  const setUrl = async () => {
    const next = urlDraft.trim();
    if (!next) return;
    setSavingUrl(true);
    setUrlError(undefined);
    try {
      await onSetLogo(next);
      setUrlDraft("");
    } catch (e) {
      setUrlError(toProblem(e).title ?? "Couldn't set that icon.");
    } finally {
      setSavingUrl(false);
    }
  };

  const pickSuggestion = async (url: string) => {
    setUrlError(undefined);
    try {
      await onSetLogo(url);
    } catch (e) {
      setUrlError(toProblem(e).title ?? "Couldn't set that icon.");
    }
  };

  const handleFile = async (file: File) => {
    setUploadError(undefined);
    if (file.size > MAX_UPLOAD_BYTES) {
      setUploadError("That file is over the 2 MB limit. Try a smaller image.");
      return;
    }
    setUploading(true);
    try {
      // The backend already sets `logo` on the channel and reconciles — this component
      // has no `logo` to adopt from the response, just a cue that the channel changed.
      await upload.mutateAsync({ id: channelId, data: { file } });
      await onSetLogo(logo ?? "");
    } catch (e) {
      setUploadError(toProblem(e).title ?? "Couldn't upload that image.");
    } finally {
      setUploading(false);
      if (fileInputRef.current) fileInputRef.current.value = "";
    }
  };

  const clear = async () => {
    setClearing(true);
    try {
      await onSetLogo("");
    } catch {
      // The parent mutation's onError toast already surfaces the failure; nothing
      // additional to show inline for a clear.
    } finally {
      setClearing(false);
    }
  };

  return (
    <div className={cn("flex flex-col gap-3", className)}>
      <div className="flex items-center gap-4">
        {/* The 64px preview — a muted placeholder box when there's no icon yet, never a
            broken-image glyph. */}
        <div className="flex size-16 shrink-0 items-center justify-center overflow-hidden rounded-md border border-border bg-static-800">
          {/*
            Three states, and the middle one is the interesting one.

            `logoImage` present ⇒ the bytes are ours (§22), so the <Image> primitive applies:
            srcset at the icon ladder, a ThumbHash while it loads, and — the reason this matters
            here specifically — a DESIGNED failure state. §22 names this field's glyph as that
            state, because an upload is the one origin that cannot be re-fetched if /data/images
            is lost, and a broken-image icon would read as a bug rather than as missing bytes.

            ⚠ `logo` without `logoImage` is an operator-pasted external URL, which stays
            supported — a plain <img> is the only honest thing to render for bytes this instance
            does not own and knows no dimensions for.
          */}
          {logoImage ? (
            <Image
              image={logoImage}
              alt="Channel icon"
              sizes="64px"
              className="size-full object-cover"
              fallback={<ImageOff className="size-6 text-static-500" aria-hidden />}
            />
          ) : logo ? (
            <img src={logo} alt="Channel icon" className="size-full object-cover" />
          ) : (
            <ImageOff className="size-6 text-static-500" aria-hidden />
          )}
        </div>
        <div className="min-w-0">
          <p className="font-medium text-sm">Channel icon</p>
          <p className="text-muted-foreground text-xs">
            Shown in your TV guide next to the channel name and number.
          </p>
        </div>
        {isAdmin && logo && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="ml-auto shrink-0 text-muted-foreground"
            disabled={clearing}
            onClick={() => void clear()}
          >
            {clearing ? <Loader2 className="size-4 animate-spin" aria-hidden /> : null}
            Clear
          </Button>
        )}
      </div>

      {isAdmin && (
        <CollapsibleSection
          title="Change icon"
          description="From your titles, upload a file, or paste an image URL"
          open={suggestionsOpen}
          onOpenChange={setSuggestionsOpen}
        >
          <div className="flex flex-col gap-4">
            {/* (a) From your titles — TMDB posters for the channel's own lineup. */}
            <div>
              <p className="font-medium text-sm">From your titles</p>
              {suggestions.isLoading && (
                <p className="mt-2 flex items-center gap-2 text-muted-foreground text-xs">
                  <Loader2 className="size-3.5 animate-spin" aria-hidden />
                  Looking for posters…
                </p>
              )}
              {tmdbUnconfigured && (
                <p className="mt-2 text-muted-foreground text-xs">
                  Set up a TMDB connection in Settings to suggest icons from this channel's titles.
                </p>
              )}
              {!suggestions.isLoading && !tmdbUnconfigured && suggestions.error && (
                <p className="mt-2 text-muted-foreground text-xs">Couldn't load suggestions right now.</p>
              )}
              {!suggestions.isLoading &&
                !tmdbUnconfigured &&
                !suggestions.error &&
                suggestionList.length === 0 && (
                  <p className="mt-2 text-muted-foreground text-xs">
                    No posters found for this channel's titles yet.
                  </p>
                )}
              {suggestionList.length > 0 && (
                <ul className="mt-2 grid grid-cols-5 gap-2 sm:grid-cols-8">
                  {suggestionList.map((s) => (
                    <li key={s.url}>
                      <button
                        type="button"
                        aria-label={`Use ${s.title}'s poster as the channel icon`}
                        title={s.title}
                        onClick={() => void pickSuggestion(s.url)}
                        className="block aspect-square w-full cursor-pointer overflow-hidden rounded-sm border border-border transition-colors hover:border-signal focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      >
                        {/*
                          ⚠ These were <img src={tmdbUrl}> until V52 phase 7 — a dozen third-party
                          requests from the operator's browser every time this popover opened,
                          which §22 names as one of the three defects the image service exists to
                          remove. The bytes are now ours, so the primitive applies: srcset at the
                          icon ladder and a ThumbHash while each loads.

                          `image` is always present here in practice — the backend only offers a
                          suggestion once its bytes have landed — but it stays optional in the DTO,
                          so the plain <img> remains as the honest fallback rather than an
                          assertion that would render nothing if the record went missing.
                        */}
                        {s.image ? (
                          <Image
                            image={s.image}
                            alt={s.title}
                            sizes="(min-width: 640px) 6rem, 4rem"
                            className="size-full object-cover"
                            fallback={<ImageOff className="size-4 text-static-500" aria-hidden />}
                          />
                        ) : (
                          <img src={s.url} alt={s.title} className="size-full object-cover" loading="lazy" />
                        )}
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            {/* (b) Upload a file. */}
            <div>
              <Label htmlFor={`${urlInputId}-file`} className="font-medium text-sm">
                Upload a file
              </Label>
              <div className="mt-2 flex items-center gap-2">
                <input
                  ref={fileInputRef}
                  id={`${urlInputId}-file`}
                  type="file"
                  accept="image/png,image/jpeg,image/webp,image/gif"
                  disabled={uploading}
                  onChange={(e) => {
                    const file = e.target.files?.[0];
                    if (file) void handleFile(file);
                  }}
                  className="block w-full text-muted-foreground text-sm file:mr-3 file:cursor-pointer file:rounded-md file:border-0 file:bg-static-800 file:px-3 file:py-1.5 file:font-medium file:text-foreground file:text-sm hover:file:bg-static-700"
                />
                {uploading && (
                  <Loader2 className="size-4 shrink-0 animate-spin text-muted-foreground" aria-hidden />
                )}
              </div>
              <p className="mt-1 text-static-400 text-xs">PNG, JPEG, WebP, or GIF, up to 2 MB.</p>
              {uploadError && <p className="mt-1 text-onair-300 text-xs">{uploadError}</p>}
            </div>

            {/* (c) Paste a direct image URL. */}
            <div>
              <Label htmlFor={urlInputId} className="font-medium text-sm">
                Paste an image URL
              </Label>
              <div className="mt-2 flex items-center gap-2">
                <Input
                  id={urlInputId}
                  type="url"
                  inputMode="url"
                  placeholder="https://example.com/icon.png"
                  value={urlDraft}
                  disabled={savingUrl}
                  onChange={(e) => {
                    setUrlDraft(e.target.value);
                    setUrlError(undefined);
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      void setUrl();
                    }
                  }}
                  className="max-w-sm"
                />
                <Button
                  type="button"
                  size="sm"
                  disabled={savingUrl || urlDraft.trim() === ""}
                  onClick={() => void setUrl()}
                >
                  {savingUrl ? (
                    <Loader2 className="size-4 animate-spin" aria-hidden />
                  ) : (
                    <Upload className="size-4" aria-hidden />
                  )}
                  Set
                </Button>
              </div>
              {urlError && <p className="mt-1 text-onair-300 text-xs">{urlError}</p>}
            </div>
          </div>
        </CollapsibleSection>
      )}
    </div>
  );
};

export { ChannelIconField };
