interface ChannelIconFieldProps {
  // Which channel this is for — drives the icon-suggestions fetch and the upload URL.
  channelId: string;
  // The channel's current icon URL (`ChannelDTO.logo`). Undefined/empty renders the
  // muted placeholder instead of a broken `<img>`.
  logo?: string;
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
