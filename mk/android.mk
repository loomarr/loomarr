## ---- Android TV React Native release ------------------------------------

.PHONY: android
android: android-release-test ## React Native Android TV — verified unsigned four-ABI Play artifact

.PHONY: android-release-test
android-release-test: ## compile once, strip the ephemeral CI signature, and retain promotion evidence
	@./scripts/test-android-release.sh

.PHONY: android-profile
android-profile: ## profile the unchanged Android gate with separate timing and memory evidence
	@node web/scripts/profile-android-build.mjs "$(or $(ANDROID_BUILD_PROFILE_DIR),$(CURDIR)/.artifacts/android-build-profile)" -- $(MAKE) android
