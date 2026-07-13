# Emby (media server) contract — Phase 0 findings

Captured 2026-07-13 against live **Emby v4.10.0.17** ("Fictional Emby"), via Tailscale direct
to `:8096`. Flavor for Phase 0 = Emby; Jellyfin path built later against §6 + this evidence.

## Auth headers — both the §6 split style AND Seerr's unified style work

| Header | Request | Result |
| --- | --- | --- |
| `X-Emby-Token: <key>` (design §6 Emby style) | `GET /Users`, `/System/Info`, `/Items` | **200** |
| `Authorization: MediaBrowser Client=…, Device=…, DeviceId=…, Version="1.0.0", Token="<key>"` (Seerr style) | `GET /Users` | **200** |

**Both authenticate on Emby 4.10.** The design doc (§6) prescribes the split approach
(`X-Emby-Token` for Emby / `Authorization: MediaBrowser Token=…` for Jellyfin). Seerr
(`server/api/jellyfin.ts`) instead uses ONE unified `Authorization: MediaBrowser …` header for
both flavors, appending `Token="…"` when authenticated, with the only flavor difference being
`Version` (`"1.0.0"` for Emby, app version for Jellyfin). **Both are valid.** The unified header
would give us a single code path — but adopting it is a §6 change and must be doc-first per the
prime directive. For now the adapter follows §6 (split); this option is recorded, not taken.

## AuthenticateByName (for §11 user login) — header shape pinned

- Endpoint: `POST /Users/AuthenticateByName`, body `{"Username","Pw"}`.
- Requires the `X-Emby-Authorization` (Emby) / `Authorization: MediaBrowser …` (Jellyfin,
  even without a token) header carrying `Client/Device/DeviceId/Version`.
- Verified: bad password with a valid header → **401** + plaintext body
  `"Invalid username or password. Please try again."` (fixture `auth_badpw_response.json`).
  This is the exact negative-path Phase 9's auth tests assert. (No test-user password was
  provided this session, so the *success* body isn't pinned — capture at Phase 9 with a throwaway
  user, or reuse Seerr's documented `AccessToken`/`User` success shape.)

## The core §6 lookup (Phase-5 presence check)

`GET /Items?Recursive=true&IncludeItemTypes=Movie&Limit=1&AnyProviderIdEquals=tmdb.<id>`
- **Present** → `Items` has 1 entry (fixture `lookup_present.json`, tmdb.16153 = "Alice Doesn't
  Live Here Anymore"). Presence = `Items` non-empty; the id is `Items[0].Id`.
- **Absent** → `Items: []` (fixture `lookup_absent.json`). Both return HTTP 200 — presence is
  array length, never status code.
- **Casing caveat (§6):** BOTH `tmdb.16153` (lowercase, doc-specified) and `Tmdb.16153`
  (Emby's own ProviderIds key casing) return the item on 4.10 — Emby matches the provider name
  case-insensitively here. The doc's "check casing first if a known title returns empty" guard
  doesn't trigger on this version but stays as defense for other versions. Use lowercase
  `tmdb.`/`tvdb.` per §6.
- Movies carry `Imdb`, `Tmdb`, AND `Tvdb` provider ids (Emby cross-populates).

## Users list (§11 import/sync)

`GET /Users` with the admin `X-Emby-Token` → 200, array of users with
`{Id, Name, Policy.IsAdministrator, Policy.IsDisabled}` (fixture `users_list.json`, 10 users).
Exactly the fields §11 needs. Per-user item path `GET /Users/{id}/Items/Latest` → 200.

## Wizard note

`GET /System/Info/Public` is unauthenticated (200) but `LocalAddresses`/`RemoteAddresses` come
back **empty** over Tailscale — the §13 "reach the server" check should key on
`ServerName`/`Version`/`Id`, not the address arrays.

## Fixtures

`system_info_authed.json`, `users_list.json`, `lookup_present.json`, `lookup_absent.json`,
`auth_badpw_response.json`. **No design-doc deviation** (unified-header is an option to consider,
not a contradiction).
