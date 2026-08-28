## ---- Android TV client ---------------------------------------------------

.PHONY: android-tokens
android-tokens: ## regenerate the Android design tokens from the shared tokens.json
	node scripts/gen-android-tokens.mjs

.PHONY: android-tokens-verify
android-tokens-verify: android-tokens ## regenerated tokens must match committed (CI red on drift)
	@git diff --exit-code android/app/src/main/java/loomarr/media/design/LoomarrTokens.kt

.PHONY: android-load
android-load: ## report heavy local processes before starting a build
	@sh scripts/dev-load-check.sh

# ⚠ NOT --no-daemon. A reused warm daemon is ONE bounded JVM; --no-daemon starts a fresh one per
# invocation, which is worse under the rapid successive builds an agent session produces. The
# ceilings live in android/gradle.properties instead, where they bound every entry point.
.PHONY: android
android: android-tokens-verify ## Android TV client — tokens + ktlint + Android Lint + unit tests + screenshots + debug APK
# ⚠ `verifyRoborazziDebug`, NOT `testDebugUnitTest`. It runs the same unit tests AND compares the
# screenshot tests against their committed baselines. Roborazzi does nothing unless a task puts it
# in record or verify mode, so under plain `testDebugUnitTest` the screenshot tests still execute,
# capture nothing, and pass — a gate that cannot fail.
	cd android && ./gradlew ktlintCheck lintDebug verifyRoborazziDebug assembleDebug

.PHONY: android-release-test
android-release-test: ## build an ephemeral signed AAB and verify release identity, ABIs, and 16 KiB alignment
	@./scripts/test-android-release.sh

.PHONY: android-fmt
android-fmt: ## Android TV client — apply ktlint formatting
	cd android && ./gradlew ktlintFormat

.PHONY: android-screenshots
android-screenshots: ## Android TV client — re-record screenshot baselines (review the diff!)
# ⚠ Recording ACCEPTS whatever the UI currently renders — it is not verification. Run it when a
# visual change is intended, then LOOK at the resulting PNG diff before committing: a baseline
# rewritten without being read turns the gate into a rubber stamp. That is the same trap web's
# visual suite carries, where `--update-snapshots` on a crashing story cheerfully records the crash.
	cd android && ./gradlew recordRoborazziDebug

.PHONY: android-stop
android-stop: ## stop the Gradle/Kotlin daemons this module started
	cd android && ./gradlew --stop
