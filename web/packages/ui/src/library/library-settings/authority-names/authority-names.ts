import type { LibraryAuthority } from "../library-settings.type";

const AUTHORITY_NAMES: Record<LibraryAuthority, string> = {
  emby: "Emby",
  jellyfin: "Jellyfin",
  radarr: "Radarr",
  sonarr: "Sonarr",
};

export { AUTHORITY_NAMES };
