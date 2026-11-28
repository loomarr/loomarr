package store

import (
	"testing"
)

// The store conformance suite (AGENTS.md: ONE suite, TWO backends — never forked per dialect).
//
// ⚠ One SUITE, not one FILE. The rule the directives protect is that both dialects run the SAME
// assertions; nothing about it requires one file, so the cases are split by domain into
// `conformance_<domain>_test.go` files (titles, channels, jobs, ops, and so on), all in this
// package. Both backend test files (sqlite in-package, Postgres behind a build tag) call
// `RunConformance`, which runs every group.
//
// The groups also make failure output navigable: a red run reads
// `TestPostgresConformance/Filler/ClipCounts` rather than one flat list of names.

// NewStoreFunc builds a fresh, migrated, empty Store for one test. The SQLite
// and Postgres test files each supply one; RunConformance runs the SAME
// assertions against both (AGENTS.md: one suite, two backends — never forked).
type NewStoreFunc func(t *testing.T) Store

// runParallel is t.Run for a group or case that runs beside its siblings. Each case opens its own
// database through newStore, so cases share no state. RunConformance returns before the parallel
// groups finish; callers release shared fixtures with t.Cleanup, which waits for them.
func runParallel(t *testing.T, name string, test func(t *testing.T)) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		test(t)
	})
}

// RunConformance is the whole contract. Every backend must pass all of it.
func RunConformance(t *testing.T, newStore NewStoreFunc) {
	runParallel(t, "Inventory", func(t *testing.T) {
		runParallel(t, "RoundTripAndIdempotency", func(t *testing.T) { testInventoryRoundTrip(t, newStore) })
		runParallel(t, "GroundedIdentityOnly", func(t *testing.T) { testInventoryGroundedIdentity(t, newStore) })
		runParallel(t, "OriginFollowsIDs", func(t *testing.T) { testInventoryOriginFollowsIDs(t, newStore) })
		runParallel(t, "EpisodeIDsKeepTheirNumbers", func(t *testing.T) { testInventoryEpisodeIDsKeepTheirNumbers(t, newStore) })
		runParallel(t, "InferredIDsYieldToCatalogs", func(t *testing.T) { testInventoryInferredIDsYieldToCatalogs(t, newStore) })
		runParallel(t, "FounderLeavesForAnotherCatalog", func(t *testing.T) { testInventoryFounderLeavesForAnotherCatalog(t, newStore) })
		runParallel(t, "MeasurementRevision", func(t *testing.T) { testInventoryMeasurementRevision(t, newStore) })
		runParallel(t, "AnalysisRevision", func(t *testing.T) { testInventoryAnalysisRevision(t, newStore) })
		runParallel(t, "ExplicitMissing", func(t *testing.T) { testInventoryExplicitMissing(t, newStore) })
		runParallel(t, "MalformedRejected", func(t *testing.T) { testInventoryMalformedRejected(t, newStore) })
		runParallel(t, "BoundsRejected", func(t *testing.T) { testInventoryBoundsRejected(t, newStore) })
		runParallel(t, "LibraryRootDiscovery", func(t *testing.T) { testLibraryRootDiscovery(t, newStore) })
		runParallel(t, "LibraryRootByHand", func(t *testing.T) { testLibraryRootByHand(t, newStore) })
		runParallel(t, "LibraryRootCheck", func(t *testing.T) { testLibraryRootCheck(t, newStore) })
		runParallel(t, "LibraryFiles", func(t *testing.T) { testLibraryFiles(t, newStore) })
		runParallel(t, "LibraryHeldTitles", func(t *testing.T) { testLibraryHeldTitles(t, newStore) })
		runParallel(t, "LibrarySkippedFiles", func(t *testing.T) { testLibrarySkippedFiles(t, newStore) })
		runParallel(t, "LibraryItems", func(t *testing.T) { testLibraryItems(t, newStore) })
		runParallel(t, "LibraryItemsReadyAndChannels", func(t *testing.T) { testLibraryItemsReadyAndChannels(t, newStore) })
		runParallel(t, "LibraryItemProblems", func(t *testing.T) { testLibraryItemProblems(t, newStore) })
		runParallel(t, "LibraryItemBrokenCopies", func(t *testing.T) { testLibraryItemBrokenCopies(t, newStore) })
		runParallel(t, "LibraryItemCopiesWaiting", func(t *testing.T) { testLibraryItemCopiesWaiting(t, newStore) })
		runParallel(t, "LibraryItemCopiesInPlace", func(t *testing.T) { testLibraryItemCopiesInPlace(t, newStore) })
		runParallel(t, "LibraryItemIDs", func(t *testing.T) { testLibraryItemIDs(t, newStore) })
		runParallel(t, "LibraryCollections", func(t *testing.T) { testLibraryCollections(t, newStore) })
		runParallel(t, "LibraryCollectionMemberIDs", func(t *testing.T) { testLibraryCollectionMemberIDs(t, newStore) })
		runParallel(t, "LibraryCatalog", func(t *testing.T) { testLibraryCatalog(t, newStore) })
		runParallel(t, "LibraryCatalogChanges", func(t *testing.T) { testLibraryCatalogChanges(t, newStore) })
		runParallel(t, "LibraryFilmography", func(t *testing.T) { testLibraryFilmography(t, newStore) })
		runParallel(t, "LibraryPersonPhotos", func(t *testing.T) { testLibraryPersonPhotos(t, newStore) })
		runParallel(t, "LibraryItemFacts", func(t *testing.T) { testLibraryItemFacts(t, newStore) })
		runParallel(t, "LibraryItemDetail", func(t *testing.T) { testLibraryItemDetail(t, newStore) })
		runParallel(t, "SeriesEpisodeTotals", func(t *testing.T) { testSeriesEpisodeTotals(t, newStore) })
		runParallel(t, "GrowthReviews", func(t *testing.T) { testGrowthReviews(t, newStore) })
		runParallel(t, "LibraryEvidence", func(t *testing.T) { testLibraryEvidence(t, newStore) })
		runParallel(t, "LibraryEpisodes", func(t *testing.T) { testLibraryEpisodes(t, newStore) })
		runParallel(t, "LibraryTitles", func(t *testing.T) { testLibraryTitles(t, newStore) })
		runParallel(t, "LibraryReferences", func(t *testing.T) { testLibraryReferences(t, newStore) })
		runParallel(t, "LibraryAudioTracks", func(t *testing.T) { testLibraryAudioTracks(t, newStore) })
		runParallel(t, "LibrarySubtitleTracks", func(t *testing.T) { testLibrarySubtitleTracks(t, newStore) })
		runParallel(t, "LibraryNeeds", func(t *testing.T) { testLibraryNeeds(t, newStore) })
		runParallel(t, "LibraryNeedsBatch", func(t *testing.T) { testLibraryNeedsBatch(t, newStore) })
		runParallel(t, "LibraryHomeVideo", func(t *testing.T) { testLibraryHomeVideo(t, newStore) })
		runParallel(t, "LibraryHomeVideoPoster", func(t *testing.T) { testLibraryHomeVideoPoster(t, newStore) })
		runParallel(t, "LibraryNeedFrame", func(t *testing.T) { testLibraryNeedFrame(t, newStore) })
		runParallel(t, "LibraryMemberReads", func(t *testing.T) { testLibraryMemberReads(t, newStore) })
		runParallel(t, "LibraryFilters", func(t *testing.T) { testLibraryFilters(t, newStore) })
		runParallel(t, "LibraryWall", func(t *testing.T) { testLibraryWall(t, newStore) })
		runParallel(t, "LibraryWallText", func(t *testing.T) { testLibraryWallText(t, newStore) })
		runParallel(t, "LibraryEpisodeOrders", func(t *testing.T) { testLibraryEpisodeOrders(t, newStore) })
		runParallel(t, "LibraryProjection", func(t *testing.T) { testLibraryProjection(t, newStore) })
		runParallel(t, "LibraryTitleFacts", func(t *testing.T) { testLibraryTitleFacts(t, newStore) })
		runParallel(t, "LibraryFilesAndTracks", func(t *testing.T) { testLibraryFilesAndTracks(t, newStore) })
	})

	runParallel(t, "Titles", func(t *testing.T) {
		runParallel(t, "TitleRoundTrip", func(t *testing.T) { testTitleRoundTrip(t, newStore) })
		runParallel(t, "UpdateTitleProgress", func(t *testing.T) { testUpdateTitleProgress(t, newStore) })
		runParallel(t, "UpsertIsIdempotent", func(t *testing.T) { testUpsertIdempotent(t, newStore) })
		runParallel(t, "GetMissingIsNotFound", func(t *testing.T) { testGetMissing(t, newStore) })
		runParallel(t, "ListByState", func(t *testing.T) { testListByState(t, newStore) })
		runParallel(t, "AvailableSince", func(t *testing.T) { testTitlesAvailableSince(t, newStore) })
		runParallel(t, "ClaimDueTitles", func(t *testing.T) { testClaimDue(t, newStore) })
		runParallel(t, "ClaimDueConcurrent", func(t *testing.T) { testClaimConcurrent(t, newStore) })
	})

	runParallel(t, "Channels", func(t *testing.T) {
		runParallel(t, "ChannelRoundTrip", func(t *testing.T) { testChannelRoundTrip(t, newStore) })
		runParallel(t, "FillerBreaks", func(t *testing.T) { testFillerBreaks(t, newStore) })
		runParallel(t, "ChannelRevisionCAS", func(t *testing.T) { testChannelRevisionCAS(t, newStore) })
		runParallel(t, "ChannelTargetedRevisionWrites", func(t *testing.T) { testChannelTargetedRevisionWrites(t, newStore) })
		runParallel(t, "ChannelListAndDelete", func(t *testing.T) { testChannelListDelete(t, newStore) })
		runParallel(t, "ChannelOrigin", func(t *testing.T) { testChannelOrigin(t, newStore) })
		runParallel(t, "ChannelOutlivesItsPurgedJob", func(t *testing.T) { testChannelOutlivesItsPurgedJob(t, newStore) })
		runParallel(t, "ApprovalOfJobNamingAChannel", func(t *testing.T) { testApprovalOfJobNamingAChannel(t, newStore) })
		runParallel(t, "ChannelHasUnfinishedJob", func(t *testing.T) { testChannelHasUnfinishedJob(t, newStore) })
		runParallel(t, "ApprovalSupersessionIsPerChannel", func(t *testing.T) { testApprovalSupersessionIsPerChannel(t, newStore) })
		runParallel(t, "PurgeKeepsJobsAwaitingADecision", func(t *testing.T) { testPurgeKeepsJobsAwaitingADecision(t, newStore) })
		runParallel(t, "ChannelDeleteDropsImageRefs", func(t *testing.T) { testChannelDeleteDropsImageRefs(t, newStore) })
		runParallel(t, "ChannelLineupRows", func(t *testing.T) { testChannelLineupRows(t, newStore) })
		runParallel(t, "ClaimDueChannels", func(t *testing.T) { testClaimDueChannels(t, newStore) })
		runParallel(t, "ClaimDueChannelsConcurrent", func(t *testing.T) { testClaimChannelsConcurrent(t, newStore) })
		runParallel(t, "AiringHistory", func(t *testing.T) { testAiringHistory(t, newStore) })
		runParallel(t, "ScheduledAiringHistory", func(t *testing.T) { testScheduledAiringHistory(t, newStore) })
		runParallel(t, "FavouriteChannels", func(t *testing.T) { testFavouriteChannels(t, newStore) })
		runParallel(t, "RecentChannels", func(t *testing.T) { testRecentChannels(t, newStore) })
		runParallel(t, "HiddenIdeas", func(t *testing.T) { testHiddenIdeas(t, newStore) })
		runParallel(t, "UpcomingAirings", func(t *testing.T) { testUpcomingAirings(t, newStore) })
		runParallel(t, "Viewings", func(t *testing.T) { testViewings(t, newStore) })
	})

	// Jobs is not a parallel group, so that its serial cases run with no other case beside them.
	t.Run("Jobs", func(t *testing.T) {
		newStore := withJobMembers(newStore)
		runParallel(t, "DiscoveryFeedback", func(t *testing.T) { testDiscoveryFeedback(t, newStore) })
		runParallel(t, "DiscoveryFeedbackMissingChannel", func(t *testing.T) { testDiscoveryFeedbackMissingChannel(t, newStore) })
		runParallel(t, "DiscoveryFeedbackHardDelete", func(t *testing.T) { testDiscoveryFeedbackHardDelete(t, newStore) })
		runParallel(t, "DiscoveryFeedbackDeleteRace", func(t *testing.T) { testDiscoveryFeedbackDeleteRace(t, newStore) })
		runParallel(t, "DiscoveryFeedbackDetachedChannel", func(t *testing.T) { testDiscoveryFeedbackDetachedChannel(t, newStore) })
		runParallel(t, "JobRoundTrip", func(t *testing.T) { testJobRoundTrip(t, newStore) })
		runParallel(t, "ProposalJobListScope", func(t *testing.T) { testProposalJobListScope(t, newStore) })
		runParallel(t, "ClaimDueJobs", func(t *testing.T) { testClaimDueJobs(t, newStore) })
		runParallel(t, "ClaimDueJobsConcurrent", func(t *testing.T) { testClaimJobsConcurrent(t, newStore) })
		runParallel(t, "JobCacheByIntentHash", func(t *testing.T) { testJobCacheByHash(t, newStore) })
		runParallel(t, "SuggestionSuccessAtomic", func(t *testing.T) { testSuggestionSuccessAtomic(t, newStore) })
		runParallel(t, "SuggestionReviseCAS", func(t *testing.T) { testSuggestionReviseCAS(t, newStore) })
		runParallel(t, "ProposalRevisionLifecycle", func(t *testing.T) { testProposalRevisionLifecycle(t, newStore) })
		runParallel(t, "ProposalRevisionFallbackApproval", func(t *testing.T) { testProposalRevisionFallbackApproval(t, newStore) })
		runParallel(t, "ProposalRevisionFallbackDenial", func(t *testing.T) { testProposalRevisionFallbackDenial(t, newStore) })
		runParallel(t, "CloneSuggestionSuccess", func(t *testing.T) { testCloneSuggestionSuccess(t, newStore) })
		runParallel(t, "CreateSuggestionResult", func(t *testing.T) { testCreateSuggestionResult(t, newStore) })
		runParallel(t, "ProposalJobSnapshot", func(t *testing.T) { testProposalJobSnapshot(t, newStore) })
		runParallel(t, "ProposalJobFirstLiveMonotonic", func(t *testing.T) { testProposalJobFirstLiveMonotonic(t, newStore) })
		runParallel(t, "ScheduledJobRoundTrip", func(t *testing.T) { testScheduledJobRoundTrip(t, newStore) })
		runParallel(t, "ScheduledJobPaused", func(t *testing.T) { testScheduledJobPaused(t, newStore) })
		runParallel(t, "ProposalRoundTripAndQueues", func(t *testing.T) { testProposalQueues(t, newStore) })
		runParallel(t, "ProposalPeople", func(t *testing.T) { testProposalPeople(t, newStore) })
		runParallel(t, "ProposalApprovalAtomic", func(t *testing.T) { testProposalApprovalAtomic(t, newStore) })
		runParallel(t, "ProposalApprovalStaleChannel", func(t *testing.T) { testProposalApprovalStaleChannel(t, newStore) })
		runParallel(t, "ProposalApprovalSuperseded", func(t *testing.T) { testProposalApprovalSuperseded(t, newStore) })
		runParallel(t, "ProposalApprovalSameIntentConflict", func(t *testing.T) { testProposalApprovalSameIntentConflict(t, newStore) })
		runParallel(t, "ProposalDecisionConcurrent", func(t *testing.T) { testProposalDecisionConcurrent(t, newStore) })
		runParallel(t, "ProposalApprovalOverlappingTitles", func(t *testing.T) { testProposalApprovalOverlappingTitles(t, newStore) })
		// Serial: a timed window must see a bypass, and a loaded process lets one slip past it.
		t.Run("ProposalAutoApprovalQuotaConcurrent", func(t *testing.T) { testProposalAutoApprovalQuotaConcurrent(t, newStore) })
		// Serial: as above.
		t.Run("ProposalApprovalManualOrdering", func(t *testing.T) { testProposalApprovalManualParticipatesInRequesterOrdering(t, newStore) })
		// Serial: as above.
		t.Run("ProposalApprovalCanceledWaiter", func(t *testing.T) { testProposalApprovalCanceledWaiterReleasesOrdering(t, newStore) })
		runParallel(t, "LookupByNonID", func(t *testing.T) { testLookupByNonID(t, newStore) })
	})

	runParallel(t, "Holidays", func(t *testing.T) {
		runParallel(t, "SeededOnOpen", func(t *testing.T) { testHolidaysSeededOnOpen(t, newStore) })
		runParallel(t, "FollowSeasonalTaxa", func(t *testing.T) { testHolidaysFollowSeasonalTaxa(t, newStore) })
	})

	runParallel(t, "BroadcastStations", func(t *testing.T) {
		runParallel(t, "SeededOnOpen", func(t *testing.T) { testBroadcastStationsSeededOnOpen(t, newStore) })
	})

	runParallel(t, "Notifications", func(t *testing.T) {
		runParallel(t, "LifecycleAndIdempotency", func(t *testing.T) { testNotificationLifecycle(t, newStore) })
		runParallel(t, "ProductVocabulary", func(t *testing.T) { testNotificationProductVocabulary(t, newStore) })
		runParallel(t, "Destinations", func(t *testing.T) { testNotificationDestinations(t, newStore) })
		runParallel(t, "ExpiredLeaseIsAmbiguous", func(t *testing.T) { testNotificationExpiredLease(t, newStore) })
		runParallel(t, "ConcurrentClaim", func(t *testing.T) { testNotificationConcurrentClaim(t, newStore) })
		runParallel(t, "Retention", func(t *testing.T) { testNotificationRetention(t, newStore) })
	})

	runParallel(t, "SecretProtection", func(t *testing.T) {
		runParallel(t, "InstallationKeyFingerprint", func(t *testing.T) {
			testInstallationKeyFingerprint(t, newStore)
		})
		runParallel(t, "InitialDataKeyIsIdempotent", func(t *testing.T) {
			testInitialSecretDataKey(t, newStore)
		})
		runParallel(t, "RotationKeepsPriorKeysReadable", func(t *testing.T) {
			testRotateSecretDataKey(t, newStore)
		})
		runParallel(t, "SettingRewritePreservesAudit", func(t *testing.T) {
			testRewriteSettingValuesPreservesAudit(t, newStore)
		})
		runParallel(t, "InstallationKeyReplacementIsAtomic", func(t *testing.T) {
			testReplaceWrappedDataKeysIsAtomic(t, newStore)
		})
	})

	runParallel(t, "Invitations", func(t *testing.T) {
		runParallel(t, "ReserveAndListIdentity", func(t *testing.T) { testInvitationReserveAndList(t, newStore) })
		runParallel(t, "ReservationBlocksDirectCreation", func(t *testing.T) {
			testInvitationReservationBlocksDirectCreation(t, newStore)
		})
		runParallel(t, "ContactIsAtomicAndGloballyUnique", func(t *testing.T) {
			testInvitationContactIsAtomicAndGloballyUnique(t, newStore)
		})
		runParallel(t, "RegenerateAndRevokeGrants", func(t *testing.T) {
			testInvitationRegenerateAndRevoke(t, newStore)
		})
		runParallel(t, "GrantPreviewDoesNotConsume", func(t *testing.T) {
			testInvitationGrantPreviewDoesNotConsume(t, newStore)
		})
		runParallel(t, "GrantPreviewRejectsEveryUnusableState", func(t *testing.T) {
			testInvitationGrantPreviewRejectsEveryUnusableState(t, newStore)
		})
		runParallel(t, "ConcurrentRedemption", func(t *testing.T) {
			testInvitationConcurrentRedemption(t, newStore)
		})
		runParallel(t, "SiblingGrantLifecycle", func(t *testing.T) {
			testInvitationSiblingGrantLifecycle(t, newStore)
		})
		runParallel(t, "Retention", func(t *testing.T) { testInvitationRetention(t, newStore) })
	})
	runParallel(t, "PasswordRecovery", func(t *testing.T) {
		runParallel(t, "PreviewDoesNotConsume", func(t *testing.T) {
			testPasswordRecoveryPreviewDoesNotConsume(t, newStore)
		})
		runParallel(t, "RedemptionIsAtomic", func(t *testing.T) {
			testPasswordRecoveryRedemptionIsAtomic(t, newStore)
		})
		runParallel(t, "NewRequestSupersedesOld", func(t *testing.T) {
			testPasswordRecoveryNewRequestSupersedesOld(t, newStore)
		})
		runParallel(t, "Retention", func(t *testing.T) {
			testPasswordRecoveryRetention(t, newStore)
		})
		runParallel(t, "DisabledAfterIssueIsUnusable", func(t *testing.T) {
			testPasswordRecoveryDisabledAfterIssueIsUnusable(t, newStore)
		})
		runParallel(t, "ConcurrentRedemption", func(t *testing.T) {
			testPasswordRecoveryConcurrentRedemption(t, newStore)
		})
	})

	// The filler suite runs in internal/filler/records against the extended store, the value the app
	// holds (#1747).

	runParallel(t, "Ops", func(t *testing.T) {
		runParallel(t, "DiscoveryQualityLedger", func(t *testing.T) { testDiscoveryQualityLedger(t, newStore) })
		runParallel(t, "SettingsKV", func(t *testing.T) { testSettings(t, newStore) })
		runParallel(t, "SessionLifecycle", func(t *testing.T) { testSessionLifecycle(t, newStore) })
		runParallel(t, "DeviceCodeMode", func(t *testing.T) { testDeviceCodeMode(t, newStore) })
		runParallel(t, "RedeemDevicePairing", func(t *testing.T) { testRedeemDevicePairing(t, newStore) })
		runParallel(t, "RedeemDevicePairingConcurrent", func(t *testing.T) { testRedeemDevicePairingConcurrent(t, newStore) })
		runParallel(t, "LastSeen", func(t *testing.T) { testLastSeen(t, newStore) })
		runParallel(t, "UserCredentialCapabilities", func(t *testing.T) { testUserCredentialCapabilities(t, newStore) })
		runParallel(t, "UserContactAddresses", func(t *testing.T) { testUserContactAddresses(t, newStore) })
		runParallel(t, "ObservabilityCounts", func(t *testing.T) { testCounts(t, newStore) })
		runParallel(t, "ActivityFeed", func(t *testing.T) { testActivityFeed(t, newStore) })
		runParallel(t, "Diagnostics", func(t *testing.T) { testDiagnostics(t, newStore) })
		runParallel(t, "DiagnosticRetentionOrder", func(t *testing.T) { testDiagnosticRetentionOrder(t, newStore) })
		runParallel(t, "RetentionPurge", func(t *testing.T) { testRetentionPurge(t, newStore) })
		runParallel(t, "SpentRowPurges", func(t *testing.T) { testSpentRowPurges(t, newStore) })
		runParallel(t, "UnusedTitlePurge", func(t *testing.T) { testUnusedTitlePurge(t, newStore) })
		runParallel(t, "CatalogRetention", func(t *testing.T) { testCatalogRetention(t, newStore) })
		runParallel(t, "ClientCrashReports", func(t *testing.T) { testClientCrashReports(t, newStore) })
		runParallel(t, "ClientCrashRetention", func(t *testing.T) { testClientCrashRetention(t, newStore) })
		runParallel(t, "TableRows", func(t *testing.T) { testTableRows(t, newStore) })
	})

	runParallel(t, "Images", func(t *testing.T) {
		runParallel(t, "ImageRoundTrip", func(t *testing.T) { testImageRoundTrip(t, newStore) })
		runParallel(t, "ImageMissingIsNotFound", func(t *testing.T) { testImageMissingIsNotFound(t, newStore) })
		runParallel(t, "ImageUpsertPreservesCreatedAt", func(t *testing.T) { testImageUpsertPreservesCreatedAt(t, newStore) })
		runParallel(t, "ImageRefsAndOwnerLookup", func(t *testing.T) { testImageRefsAndOwnerLookup(t, newStore) })
		runParallel(t, "ImageOrphanDetection", func(t *testing.T) { testImageOrphanDetection(t, newStore) })
		runParallel(t, "ImageDerivatives", func(t *testing.T) { testImageDerivatives(t, newStore) })
		runParallel(t, "ImageDerivativeBatchAtomic", func(t *testing.T) { testImageDerivativeBatchAtomic(t, newStore) })
		runParallel(t, "ImagesMissingFormat", func(t *testing.T) { testImagesMissingFormat(t, newStore) })
		runParallel(t, "ImageFetchQueueAndExpirySweepAreDisjoint", func(t *testing.T) { testImageFetchQueueAndExpirySweepAreDisjoint(t, newStore) })
		runParallel(t, "ImageExpiryPagesThroughEverythingDue", func(t *testing.T) { testImageExpiryPagesThroughEverythingDue(t, newStore) })
		runParallel(t, "ImageFetchFailureBacksOff", func(t *testing.T) { testImageFetchFailureBacksOff(t, newStore) })
		runParallel(t, "ImagesByHash", func(t *testing.T) { testImagesByHash(t, newStore) })
		runParallel(t, "ImageUnrecoverableSelection", func(t *testing.T) { testImageUnrecoverableSelection(t, newStore) })
		runParallel(t, "ImageDeleteCascades", func(t *testing.T) { testImageDeleteCascades(t, newStore) })
		runParallel(t, "ImageTouch", func(t *testing.T) { testImageTouch(t, newStore) })
		runParallel(t, "ImagesByOrigin", func(t *testing.T) { testImagesByOrigin(t, newStore) })
		runParallel(t, "ImageBySourceURL", func(t *testing.T) { testImageBySourceURL(t, newStore) })
		runParallel(t, "ImageFetchQueueOrderIsWhenQueued", func(t *testing.T) { testImageFetchQueueOrderIsWhenQueued(t, newStore) })
		runParallel(t, "ImagePendingReadsBackWithNoFetchTime", func(t *testing.T) { testImagePendingReadsBackWithNoFetchTime(t, newStore) })
		runParallel(t, "ImageRefsRepoint", func(t *testing.T) { testImageRefsRepoint(t, newStore) })
		runParallel(t, "ImageDerivativeBudgetAndEvictionOrder", func(t *testing.T) {
			testImageDerivativeBudgetAndEvictionOrder(t, newStore)
		})
	})
}
