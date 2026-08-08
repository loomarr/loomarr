// Barrel for @loomarr/api: orval-generated TanStack Query hooks (namespaced per
// domain so the fetch-client helper enums orval repeats in every tag file don't
// collide), plus the flat model types and the shared transport mutator.
//
//   import { setupApi, type SetupCheck, ApiError } from "@loomarr/api";
//   const { data } = setupApi.useSetupStatus();
export * as authApi from "../generated/endpoints/auth/auth";
export * as channelsApi from "../generated/endpoints/channels/channels";
export * as dashboardApi from "../generated/endpoints/dashboard/dashboard";
export * as fillerApi from "../generated/endpoints/filler/filler";
export * as helpApi from "../generated/endpoints/help/help";
export * as jobsApi from "../generated/endpoints/jobs/jobs";
export * as libraryApi from "../generated/endpoints/library/library";
export * as playoutApi from "../generated/endpoints/playout/playout";
export * as proposalsApi from "../generated/endpoints/proposals/proposals";
export * as searchApi from "../generated/endpoints/search/search";
export * as settingsApi from "../generated/endpoints/settings/settings";
export * as setupApi from "../generated/endpoints/setup/setup";
export * as systemApi from "../generated/endpoints/system/system";
export * as titlesApi from "../generated/endpoints/titles/titles";
export * as usersApi from "../generated/endpoints/users/users";
export * from "../generated/model";
export * from "./mutator";
export * from "./unwrap";
