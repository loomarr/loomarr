import type { ImageDTO } from "@loomarr/api/models/imageDTO";

interface ChannelIconFieldProps {
  // Which channel this is for — drives the icon-suggestions fetch and the upload URL.
  channelId: string;
  // The channel's current icon URL (`ChannelDTO.logo`). Undefined/empty renders the
  // muted placeholder instead of a broken `<img>`.
  logo?: string;
  // The image record when `logo` is served by this instance's image service
  // (`ChannelDTO.logoImage`, §22). Present ⇒ the preview renders through the `<Image>`
  // primitive and gets srcset, a ThumbHash and a designed failure state.
  //
  // ⚠ Absent is NOT an error state — an operator may paste any URL as a channel icon, and
  // that stays supported. The preview falls back to a plain `<img>` on that URL, which is
  // the only thing it can do for bytes this instance does not own.
  logoImage?: ImageDTO;
  // Commit a new `logo` value (including `""` to clear it) through the channel's own
  // PATCH mutation (§7) — the SAME mutation the identity fields use, so this is just
  // another field on that one update, not a bespoke endpoint. Resolves on success;
  // the caller's mutation-level onSuccess/onError (toast + invalidate) already covers
  // this field, so this component doesn't toast on the PATCH path itself.
  onSetLogo: (logo: string) => Promise<void>;
  // A viewer sees the current icon but none of the editing affordances — only an
  // admin can change what's broadcast (§7 authorization model). Matches the `isAdmin`
  // gate already used by AppShell/ChannelDangerZone/etc.
  isAdmin?: boolean;
  className?: string;
}

export type { ChannelIconFieldProps };
