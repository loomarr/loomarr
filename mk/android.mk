## ---- Android TV React Native release ------------------------------------

.PHONY: android
android: android-release-test ## React Native Android TV — signed four-ABI Play bundle verification

.PHONY: android-release-test
android-release-test: ## build an ephemeral signed React Native AAB and verify identity, ABIs, and 16 KiB alignment
	@./scripts/test-android-release.sh
