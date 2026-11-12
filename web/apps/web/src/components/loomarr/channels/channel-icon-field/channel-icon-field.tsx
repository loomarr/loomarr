import * as channelsApi from "@loomarr/api/endpoints/channels";
import { ApiError, toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { Action, ActivityIndicator, Field, Icon, icons, Stack, Surface, Text } from "@loomarr/design-system";
import { useStore } from "@tanstack/react-form";
import { useId, useRef, useState } from "react";
import { ServedImage } from "@/components/loomarr/served-image";
import { useAppForm } from "@/lib/app-form";
import type { ChannelIconFieldProps } from "./channel-icon-field.type";

// ⚠ The multipart upload is NOT hand-written here. POST …/icon is a Huma operation with a
// huma.MultipartFormFiles body, so orval generates the call — it builds the FormData itself,
// and customFetch adds CSRF and credentials without touching the body (the mutator passes
// `options.body` straight through to fetch; it does not JSON-encode it). Re-implementing the
// transport contract here (same-origin, `credentials: "include"`, the double-submit CSRF
// header) would only be a second copy to drift.
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
  const pickerId = useId();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [urlError, setUrlError] = useState<string>();
  // The pasted address stays in the box when the save fails, so it can be corrected.
  const urlForm = useAppForm({
    defaultValues: { url: "" },
    onSubmit: async ({ value, formApi }) => {
      const next = value.url.trim();
      if (!next) return;
      setUrlError(undefined);
      try {
        await onSetLogo(next);
        formApi.reset();
      } catch (e) {
        setUrlError(toProblem(e).title ?? "Couldn't set that icon.");
      }
    },
  });
  const urlDraft = useStore(urlForm.store, (state) => state.values.url);
  const savingUrl = useStore(urlForm.store, (state) => state.isSubmitting);

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
    <Stack className={className} gap="control">
      <Stack align="center" direction="row" gap="control">
        {/* The web mock's 48px tile: a muted placeholder when there's no icon yet, never a
            broken-image glyph. */}
        <div className="border-border bg-background flex size-12 shrink-0 items-center justify-center overflow-hidden rounded-md border">
          {/*
            Three states, and the middle one is the interesting one.

            `logoImage` present ⇒ the bytes are ours (§22), so the <ServedImage> primitive applies:
            srcset at the icon ladder, a ThumbHash while it loads, and — the reason this matters
            here specifically — a DESIGNED failure state. §22 names this field's glyph as that
            state, because an upload is the one origin that cannot be re-fetched if /data/images
            is lost, and a broken-image icon would read as a bug rather than as missing bytes.

            ⚠ `logo` without `logoImage` is an operator-pasted external URL, which stays
            supported — a plain <img> is the only honest thing to render for bytes this instance
            does not own and knows no dimensions for.
          */}
          {logoImage ? (
            <ServedImage
              image={logoImage}
              alt="Channel icon"
              sizes="48px"
              fallback={<Icon decorative glyph={icons.noImage} size="control" tone="muted" />}
            />
          ) : logo ? (
            <img src={logo} alt="Channel icon" className="size-full object-cover" />
          ) : (
            <Icon decorative glyph={icons.noImage} size="control" tone="muted" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          <Text as="p" textRole="cardLabel" tone="primary">
            Channel icon
          </Text>
          <Text as="p" className="mt-0.5" textRole="hint" tone="secondary">
            What your household sees in the media server's guide.
          </Text>
        </div>
        {isAdmin && (
          <Action
            aria-controls={pickerId}
            aria-expanded={suggestionsOpen}
            onPress={() => setSuggestionsOpen((open) => !open)}
            style={{ flexShrink: 0 }}
            variant="outline"
          >
            Change…
          </Action>
        )}
      </Stack>

      {/* The mock draws only the "Change…" button, not what it opens, so the picker keeps
          today's three ways in plus Clear. */}
      {isAdmin && suggestionsOpen && (
        <div id={pickerId} className="border-border rounded-md border p-4">
          <Stack gap="4">
            {logo && (
              <Action
                disabled={clearing}
                onPress={() => void clear()}
                style={{ alignSelf: "flex-start" }}
                variant="ghost"
              >
                {clearing ? "Clearing…" : "Clear"}
              </Action>
            )}
            {/* (a) From your titles — TMDB posters for the channel's own lineup. */}
            <div>
              <Text as="p" textRole="cardLabel" tone="primary">
                From your titles
              </Text>
              {suggestions.isLoading && (
                <Stack align="center" className="mt-2" direction="row" gap="inline" render="p">
                  <ActivityIndicator decorative size="compact" tone="secondary" />
                  <Text as="span" textRole="hint" tone="secondary">
                    Looking for posters…
                  </Text>
                </Stack>
              )}
              {tmdbUnconfigured && (
                <Text as="p" className="mt-2" textRole="hint" tone="secondary">
                  Set up a TMDB connection in Settings to suggest icons from this channel's titles.
                </Text>
              )}
              {!suggestions.isLoading && !tmdbUnconfigured && suggestions.error && (
                <Text as="p" className="mt-2" textRole="hint" tone="secondary">
                  Couldn't load suggestions right now.
                </Text>
              )}
              {!suggestions.isLoading &&
                !tmdbUnconfigured &&
                !suggestions.error &&
                suggestionList.length === 0 && (
                  <Text as="p" className="mt-2" textRole="hint" tone="secondary">
                    No posters found for this channel's titles yet.
                  </Text>
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
                        className="border-border hover:border-static-500 focus-visible:ring-ring block aspect-square w-full cursor-pointer overflow-hidden rounded-sm border transition-colors focus-visible:ring-2 focus-visible:outline-none"
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
                          <ServedImage
                            image={s.image}
                            alt={s.title}
                            sizes="(min-width: 640px) 6rem, 4rem"
                            fallback={<Icon decorative glyph={icons.noImage} size="compact" tone="muted" />}
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
              <Text as="label" htmlFor={`${urlInputId}-file`} textRole="label">
                Upload a file
              </Text>
              <Stack align="center" className="mt-2" direction="row" gap="inline">
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
                  className="text-muted-foreground file:bg-static-800 file:text-foreground hover:file:bg-static-700 block w-full text-sm file:mr-3 file:cursor-pointer file:rounded-md file:border-0 file:px-3 file:py-1.5 file:text-sm file:font-medium"
                />
                {uploading && <ActivityIndicator decorative size="compact" tone="muted" />}
              </Stack>
              <Text as="p" className="mt-1" textRole="hint" tone="secondary">
                PNG, JPEG, WebP, or GIF, up to 2 MB.
              </Text>
              {uploadError && (
                <Text as="p" className="mt-1" textRole="hint" tone="danger">
                  {uploadError}
                </Text>
              )}
            </div>

            {/* (c) Paste a direct image URL. */}
            <div>
              <Text as="label" htmlFor={urlInputId} textRole="label">
                Paste an image URL
              </Text>
              <Stack align="center" className="mt-2" direction="row" gap="inline">
                <Surface
                  backgroundColor="transparent"
                  borderWidth={0}
                  flexGrow={1}
                  flexShrink={1}
                  maxWidth={384}
                  minWidth={0}
                >
                  <urlForm.Field name="url">
                    {(field) => (
                      <Field
                        autoCapitalize="none"
                        autoCorrect={false}
                        disabled={savingUrl}
                        id={urlInputId}
                        keyboardType="url"
                        onChangeText={(text) => {
                          field.handleChange(text);
                          setUrlError(undefined);
                        }}
                        onSubmitEditing={() => void urlForm.handleSubmit()}
                        placeholder="https://example.com/icon.png"
                        value={field.state.value}
                      />
                    )}
                  </urlForm.Field>
                </Surface>
                <Action
                  disabled={savingUrl || urlDraft.trim() === ""}
                  onPress={() => void urlForm.handleSubmit()}
                >
                  {savingUrl ? "Setting…" : "Set"}
                </Action>
              </Stack>
              {urlError && (
                <Text as="p" className="mt-1" textRole="hint" tone="danger">
                  {urlError}
                </Text>
              )}
            </div>
          </Stack>
        </div>
      )}
    </Stack>
  );
};

export { ChannelIconField };
