// Complete @loomarr/api catalog for tests and tooling. Production runtime code uses the package's
// endpoint, model, mutator, unwrap, and zod subpaths so unrelated generated clients do not enter the
// initial route graph (frontend-design §4.4).
//
// ⚠ Generated models are TYPE-ONLY as a group. Orval emits enum fields as runtime objects, so a
// plain star export makes Vite load every DTO module before the app can start. Runtime enum objects
// the UI deliberately uses are the small explicit list below; TypeScript catches a missing one.
//
//   import * as setupApi from "@loomarr/api/endpoints/setup";
//   const { data } = setupApi.useSetupStatus();
export * as authApi from "../generated/endpoints/auth/auth";
export * as channelsApi from "../generated/endpoints/channels/channels";
export * as dashboardApi from "../generated/endpoints/dashboard/dashboard";
export * as diagnosticsApi from "../generated/endpoints/diagnostics/diagnostics";
export * as discoveryApi from "../generated/endpoints/discovery/discovery";
export * as eventsApi from "../generated/endpoints/events/events";
export * as fillerApi from "../generated/endpoints/filler/filler";
export * as helpApi from "../generated/endpoints/help/help";
export * as imagesApi from "../generated/endpoints/images/images";
export * as invitationsApi from "../generated/endpoints/invitations/invitations";
export * as jobsApi from "../generated/endpoints/jobs/jobs";
export * as libraryApi from "../generated/endpoints/library/library";
export * as notificationsApi from "../generated/endpoints/notifications/notifications";
export * as opsApi from "../generated/endpoints/ops/ops";
export * as playoutApi from "../generated/endpoints/playout/playout";
export * as proposalJobsApi from "../generated/endpoints/proposal-jobs/proposal-jobs";
export * as proposalsApi from "../generated/endpoints/proposals/proposals";
export * as searchApi from "../generated/endpoints/search/search";
export * as settingsApi from "../generated/endpoints/settings/settings";
export * as setupApi from "../generated/endpoints/setup/setup";
export * as systemApi from "../generated/endpoints/system/system";
export * as titlesApi from "../generated/endpoints/titles/titles";
export * as usersApi from "../generated/endpoints/users/users";
export type * from "../generated/model";
export { ClipDTOAudience } from "../generated/model/clipDTOAudience";
export { ClipDTOKind } from "../generated/model/clipDTOKind";
export { ExcludedItemDTOReason } from "../generated/model/excludedItemDTOReason";
export { SettingEntryProvenance } from "../generated/model/settingEntryProvenance";
export { SettingResultStatus } from "../generated/model/settingResultStatus";
export { TitleDTOState } from "../generated/model/titleDTOState";
export * from "./mutator";
export * from "./unwrap";
