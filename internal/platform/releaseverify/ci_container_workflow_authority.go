package releaseverify

type workflowStepAuthority struct {
	targets           []string
	allowsAcquisition bool
	exactName         bool
	name              string
	exactStepIndex    bool
	stepIndex         int
	environment       map[string]string
	condition         string
	shell             string
	// nonBlocking is the one explicit way a source-bound step may set continue-on-error: true
	// (and then must). Only the flake quarantine's step uses it (#1570 step 3); every other step
	// stays forbidden from tolerating its own failure.
	nonBlocking bool
}

type workflowJobAuthority struct {
	environment map[string]string
	condition   string
	permissions map[string]string
	steps       map[string]workflowStepAuthority
}

// workflowRunTrigger pins an on.workflow_run trigger to exact source workflows and run types.
type workflowRunTrigger struct {
	workflows []string
	types     []string
}

type workflowAuthority struct {
	name             string
	pullRequestTypes []string
	pushBranches     []string
	workflowRun      *workflowRunTrigger
	environment      map[string]string
	permissions      map[string]string
	jobs             map[string]workflowJobAuthority
}

func standardWorkflowEnvironment() map[string]string {
	return map[string]string{
		"GO_VERSION":   "1.27",
		"NODE_VERSION": "22",
	}
}

func standardWorkflowPermissions() map[string]string {
	return map[string]string{"contents": "read"}
}

func workflowRunAuthorityEntries() map[string]workflowAuthority {
	return map[string]workflowAuthority{
		"android-ccache-promotion.yml": {
			name:        "Android ccache promotion",
			workflowRun: &workflowRunTrigger{workflows: []string{"CI"}, types: []string{"completed"}},
			permissions: map[string]string{"actions": "write", "contents": "read"},
			jobs: map[string]workflowJobAuthority{
				"promote": {
					condition: "github.event.workflow_run.event == 'merge_group' && github.event.workflow_run.conclusion == 'success'",
					steps: map[string]workflowStepAuthority{
						`./scripts/android-ccache-promotion.sh locate >> "$GITHUB_OUTPUT"`: exactWorkflowStep(1, "Locate the exact merge-queue producer", workflowStepAuthority{environment: map[string]string{
							"GH_TOKEN": "${{ secrets.GITHUB_TOKEN }}", "REPO": "${{ github.repository }}",
							"PRODUCER_SHA": "${{ github.event.workflow_run.head_sha }}",
						}}),
						"./scripts/android-ccache-promotion.sh restore \"$RUNNER_TEMP/loomarr-android-ccache-transfer\" \"$RUNNER_TEMP/loomarr-android-ccache\" \"$PRODUCER_SHA\" \"${{ steps.producer.outputs.run-id }}\" android-tv-ccache-v2-Linux-ccache-4.14- >> \"$GITHUB_OUTPUT\"": exactWorkflowStep(3, "Validate the producer binding and cache contents", workflowStepAuthority{
							condition:   "steps.producer.outputs.found == 'true'",
							environment: map[string]string{"PRODUCER_SHA": "${{ github.event.workflow_run.head_sha }}"},
						}),
						androidCCachePromotionRetentionCommand: exactWorkflowStep(5, "Retire the transfer and superseded main cache generations", workflowStepAuthority{
							condition: "steps.producer.outputs.found == 'true'",
							environment: map[string]string{
								"GH_TOKEN": "${{ secrets.GITHUB_TOKEN }}", "REPO": "${{ github.repository }}",
								"ARTIFACT_ID": "${{ steps.producer.outputs.artifact-id }}", "KEEP_KEY": "${{ steps.cache.outputs.cache-key }}",
							},
						}),
					},
				},
			},
		},
		"android-beta.yml": {
			permissions: map[string]string{"actions": "read", "contents": "read"},
			jobs: map[string]workflowJobAuthority{
				"release": {
					condition: "github.ref == 'refs/heads/main'",
					environment: map[string]string{
						"ANDROID_RELEASE_OUTPUT_DIR":         "${{ github.workspace }}/.artifacts/android-release",
						"LOOMARR_ANDROID_KEYSTORE_PASSWORD":  "${{ secrets.ANDROID_UPLOAD_KEYSTORE_PASSWORD }}",
						"LOOMARR_ANDROID_KEY_ALIAS":          "${{ secrets.ANDROID_UPLOAD_KEY_ALIAS }}",
						"LOOMARR_ANDROID_KEY_PASSWORD":       "${{ secrets.ANDROID_UPLOAD_KEY_PASSWORD }}",
						"LOOMARR_ANDROID_UPLOAD_CERT_SHA256": "${{ vars.ANDROID_UPLOAD_CERT_SHA256 }}",
					},
				},
			},
		},
		"apple-testflight.yml": {
			permissions: map[string]string{"contents": "read"},
			jobs: map[string]workflowJobAuthority{
				"plan": {condition: appleTestFlightMain},
				"release": {
					condition: appleTestFlightReleaseCondition,
					environment: map[string]string{
						"LOOMARR_APPLE_APP":          "${{ matrix.app }}",
						"LOOMARR_APPLE_BUILD_NUMBER": "${{ github.run_number }}",
						"LOOMARR_APPLE_OUTPUT_DIR":   "${{ github.workspace }}/.artifacts/testflight",
					},
					steps: map[string]workflowStepAuthority{
						"make fe-install":     exactWorkflowStep(3, "", workflowStepAuthority{targets: []string{"fe-install"}, allowsAcquisition: true}),
						"make fe-api-codegen": exactWorkflowStep(4, "", workflowStepAuthority{targets: []string{"fe-api-codegen"}}),
					},
				},
			},
		},
		"apple-compilation-cache.yml": {
			environment: standardWorkflowEnvironment(),
			permissions: map[string]string{"actions": "write", "contents": "read"},
			jobs: map[string]workflowJobAuthority{
				"publish": {
					condition: "github.ref == 'refs/heads/main'",
					steps: map[string]workflowStepAuthority{
						appleCachePublisherPreflightCommand: exactWorkflowStep(1, "Preflight repository cache headroom", workflowStepAuthority{environment: map[string]string{
							"GH_TOKEN": "${{ secrets.GITHUB_TOKEN }}", "REPO": "${{ github.repository }}",
						}}),
						"make fe-install":              exactWorkflowStep(5, "", workflowStepAuthority{targets: []string{"fe-install"}, allowsAcquisition: true}),
						appleCacheFingerprintCommand:   exactWorkflowStep(6, "Fingerprint the Apple toolchain", workflowStepAuthority{}),
						appleCachePublisherSeedCommand: exactWorkflowStep(8, "Isolate the optional seed generation", workflowStepAuthority{}),
						`./web/scripts/validate-apple-compilation-cache.sh produce "$RUNNER_TEMP/apple-compilation-cache.tar.zst"`: exactWorkflowStep(9, "Build and validate the candidate generation", workflowStepAuthority{allowsAcquisition: true, environment: map[string]string{
							"LOOMARR_APPLE_CACHE_SEED_ARCHIVE":    "${{ steps.seed.outputs.archive }}",
							"LOOMARR_APPLE_CACHE_VALIDATION_ROOT": "${{ runner.temp }}/apple-cache-publisher",
						}}),
						appleCachePublisherAdmissionCommand: exactWorkflowStep(10, "Enforce repository cache headroom", workflowStepAuthority{environment: map[string]string{
							"GH_TOKEN": "${{ secrets.GITHUB_TOKEN }}", "REPO": "${{ github.repository }}",
						}}),
						appleCachePublisherRetentionCommand: exactWorkflowStep(12, "Retain only the newest generation for this fingerprint", workflowStepAuthority{environment: map[string]string{
							"GH_TOKEN": "${{ secrets.GITHUB_TOKEN }}", "REPO": "${{ github.repository }}", "PREFIX": "${{ steps.apple-toolchain.outputs.fingerprint }}",
						}}),
					},
				},
			},
		},
		"cache-cleanup.yml": {
			name:             "Cache cleanup",
			pullRequestTypes: []string{"closed"},
			permissions:      map[string]string{"actions": "write"},
			jobs: map[string]workflowJobAuthority{
				"cleanup": {
					steps: map[string]workflowStepAuthority{
						cacheCleanupWorkflowCommand: exactWorkflowStep(0, "Delete inaccessible caches for pull request ${{ github.event.pull_request.number }}", workflowStepAuthority{
							environment: map[string]string{
								"GH_TOKEN":         "${{ secrets.GITHUB_TOKEN }}",
								"REPO":             "${{ github.repository }}",
								"REF":              "refs/pull/${{ github.event.pull_request.number }}/merge",
								"QUEUE_REF_PREFIX": "refs/heads/gh-readonly-queue/main/pr-${{ github.event.pull_request.number }}-",
							},
						}),
					},
				},
			},
		},
		"ci-agent.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			"make agent-harness-test": exactWorkflowStep(2, "Worktree isolation, claims, leases, and process ownership", workflowStepAuthority{targets: []string{"agent-harness-test"}}),
		}),
		"ci-android.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			"make fe-install": exactWorkflowStep(5, "", workflowStepAuthority{targets: []string{"fe-install"}, allowsAcquisition: true}),
			"make fe-codegen": exactWorkflowStep(6, "", workflowStepAuthority{targets: []string{"fe-codegen"}}),
			"web/scripts/install-android-ccache.sh \"${RUNNER_TEMP}/loomarr-android-ccache-tool\"": exactWorkflowStep(7, "Install pinned ccache", workflowStepAuthority{allowsAcquisition: true}),
			"make android-profile": exactWorkflowStep(9, "Build and profile the verified four-ABI bundle", workflowStepAuthority{
				targets: []string{"android-profile", "android"},
				environment: map[string]string{
					"ANDROID_CI_OUTPUT_DIR":           "${{ github.workspace }}/.artifacts/android-ci",
					"CCACHE_DIR":                      "${{ runner.temp }}/loomarr-android-ccache",
					"CCACHE_BASEDIR":                  "${{ github.workspace }}",
					"CCACHE_MAXSIZE":                  "2G",
					"CCACHE_SLOPPINESS":               "",
					"LOOMARR_ANDROID_CCACHE_LAUNCHER": "${{ steps.ccache.outputs.launcher }}",
					"LOOMARR_ANDROID_GRADLE_WORKERS":  "2",
				},
			}),
			"./scripts/android-ccache-promotion.sh pack \"$RUNNER_TEMP/loomarr-android-ccache\" \"$RUNNER_TEMP/loomarr-android-ccache-transfer\" \"android-tv-ccache-v2-${{ runner.os }}-ccache-4.14-${{ hashFiles('web/apps/tv/**', 'web/packages/**', 'web/pnpm-lock.yaml', 'web/scripts/**') }}-${{ github.run_id }}\"": exactWorkflowStep(10, "Package the verified merge-queue compiler cache", workflowStepAuthority{
				condition: "github.event_name == 'merge_group'",
			}),
		}),
		"ci-apple-mobile.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			"make fe-install": exactWorkflowStep(3, "", workflowStepAuthority{targets: []string{"fe-install"}}),
			"cd web && pnpm -C packages/lan-discovery-native exec vitest run tests/ios-unicast-target-planner.test.mjs": exactWorkflowStep(4, "Test the iOS LAN discovery planner", workflowStepAuthority{}),
			"cd web && pnpm -C packages/crash-reports-native exec vitest run tests/ios-crash-payload-store.test.mjs":    exactWorkflowStep(5, "Test the iOS MetricKit crash payload store", workflowStepAuthority{}),
			appleCacheFingerprintCommand:     exactWorkflowStep(6, "Fingerprint the Apple toolchain", workflowStepAuthority{}),
			appleCacheConsumerPrepareCommand: exactWorkflowStep(8, "Validate the restored Apple compilation cache", workflowStepAuthority{}),
			"make client-apple-simulator CLIENT_APP=mobile": exactWorkflowStep(9, "Generate, build, install, and launch", workflowStepAuthority{
				targets: []string{"client-apple-simulator"},
				environment: map[string]string{
					"LOOMARR_APPLE_ARTIFACTS_DIR": "${{ runner.temp }}/apple-client-mobile",
					"LOOMARR_APPLE_BUILD_DIR":     "${{ runner.temp }}/apple-build-mobile",
					"LOOMARR_APPLE_CACHE_MODE":    "${{ steps.apple-compilation-cache.outputs.mode }}",
					"LOOMARR_APPLE_CACHE_STORE":   "${{ steps.apple-compilation-cache.outputs.store }}",
				},
			}),
		}),
		"ci-apple-tv.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			"make fe-install":                exactWorkflowStep(3, "", workflowStepAuthority{targets: []string{"fe-install"}}),
			appleCacheFingerprintCommand:     exactWorkflowStep(4, "Fingerprint the Apple toolchain", workflowStepAuthority{}),
			appleCacheConsumerPrepareCommand: exactWorkflowStep(6, "Validate the restored Apple compilation cache", workflowStepAuthority{}),
			"make client-apple-simulator CLIENT_APP=tv": exactWorkflowStep(7, "Generate, build, install, and launch", workflowStepAuthority{
				targets: []string{"client-apple-simulator"},
				environment: map[string]string{
					"LOOMARR_APPLE_ARTIFACTS_DIR": "${{ runner.temp }}/apple-client-tv",
					"LOOMARR_APPLE_BUILD_DIR":     "${{ runner.temp }}/apple-build-tv",
					"LOOMARR_APPLE_CACHE_MODE":    "${{ steps.apple-compilation-cache.outputs.mode }}",
					"LOOMARR_APPLE_CACHE_STORE":   "${{ steps.apple-compilation-cache.outputs.store }}",
				},
			}),
		}),
		"ci-expo-android-mobile.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			"make fe-install": exactWorkflowStep(5, "", workflowStepAuthority{targets: []string{"fe-install"}, allowsAcquisition: true}),
			"web/scripts/install-android-ccache.sh \"${RUNNER_TEMP}/loomarr-android-ccache-tool\"": exactWorkflowStep(6, "Install pinned ccache", workflowStepAuthority{allowsAcquisition: true}),
			"make client-android-debug CLIENT_APP=mobile": exactWorkflowStep(7, "Generate and build the standalone mobile APK", workflowStepAuthority{
				targets: []string{"client-android-debug"},
				environment: map[string]string{
					"CCACHE_DIR":                      "${{ runner.temp }}/loomarr-android-ccache",
					"CCACHE_BASEDIR":                  "${{ github.workspace }}",
					"CCACHE_MAXSIZE":                  "2G",
					"CCACHE_SLOPPINESS":               "",
					"LOOMARR_ANDROID_CCACHE_LAUNCHER": "${{ steps.ccache.outputs.launcher }}",
					"LOOMARR_ANDROID_GRADLE_WORKERS":  "2",
					"LOOMARR_ANDROID_NATIVE_JOBS":     "4",
					"LOOMARR_ANDROID_METRO_WORKERS":   "4",
					"LOOMARR_ANDROID_GRADLE_HEAP":     "3g",
					"LOOMARR_ANDROID_MEMORY_HIGH":     "11G",
					"LOOMARR_ANDROID_MEMORY_MAX":      "12G",
				},
			}),
		}),
		"ci-apple-cache-validation.yml": {
			environment: standardWorkflowEnvironment(),
			permissions: standardWorkflowPermissions(),
			jobs: map[string]workflowJobAuthority{
				"producer": {steps: map[string]workflowStepAuthority{
					"make fe-install": exactWorkflowStep(4, "", workflowStepAuthority{targets: []string{"fe-install"}, allowsAcquisition: true}),
					`./web/scripts/validate-apple-compilation-cache.sh produce "$RUNNER_TEMP/apple-compilation-cache.tar.zst"`: exactWorkflowStep(5, "Produce and validate the compilation cache", workflowStepAuthority{allowsAcquisition: true, environment: map[string]string{"LOOMARR_APPLE_CACHE_VALIDATION_ROOT": "${{ runner.temp }}/apple-cache-validation-producer"}}),
				}},
				"consumer": {steps: map[string]workflowStepAuthority{
					"make fe-install": exactWorkflowStep(4, "", workflowStepAuthority{targets: []string{"fe-install"}, allowsAcquisition: true}),
					`./web/scripts/validate-apple-compilation-cache.sh consume "$RUNNER_TEMP/apple-compilation-cache.tar.zst"`: exactWorkflowStep(6, "Consume and invalidate the compilation cache", workflowStepAuthority{allowsAcquisition: true, environment: map[string]string{"LOOMARR_APPLE_CACHE_VALIDATION_ROOT": "${{ runner.temp }}/apple-cache-validation-consumer"}}),
				}},
			},
		},
		"ci-clients.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			"make fe-install": exactWorkflowStep(5, "", workflowStepAuthority{targets: []string{"fe-install"}}),
			"make clients-checks": exactWorkflowStep(6, "Verify the affected client graph", workflowStepAuthority{
				targets:   []string{"clients-checks"},
				condition: "matrix.lane == 'checks'",
				environment: map[string]string{
					"TMPDIR":                   "${{ runner.temp }}",
					"TURBO_TELEMETRY_DISABLED": "1",
				},
			}),
			"make clients-tests CLIENTS_APP=${{ matrix.lane == 'tv-tests' && 'tv' || 'mobile' }}": exactWorkflowStep(7, "Typecheck and test the client", workflowStepAuthority{
				targets:   []string{"clients-tests"},
				condition: "endsWith(matrix.lane, '-tests')",
				environment: map[string]string{
					"TMPDIR":                   "${{ runner.temp }}",
					"TURBO_TELEMETRY_DISABLED": "1",
				},
			}),
			"make clients-bundles CLIENTS_APP=${{ matrix.lane }}": exactWorkflowStep(8, "Bundle the client", workflowStepAuthority{
				targets:   []string{"clients-bundles"},
				condition: "matrix.lane == 'mobile' || matrix.lane == 'tv'",
				environment: map[string]string{
					"TMPDIR":                   "${{ runner.temp }}",
					"TURBO_TELEMETRY_DISABLED": "1",
				},
			}),
		}),
		"ci-docs.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			// Pulls the digest-pinned D2 image (mk/docs.mk); the render itself runs with --network none.
			"make diagrams-verify": exactWorkflowStep(7, "Diagrams", workflowStepAuthority{targets: []string{"diagrams-verify"}, condition: "inputs.diagrams", allowsAcquisition: true}),
		}),
		"ci-frontend.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			"make fe-install": exactWorkflowStep(4, "", workflowStepAuthority{targets: []string{"fe-install"}}),
			"make fe-unit FE_SHARD=${{ matrix.shard }}/6": exactWorkflowStep(5, "", workflowStepAuthority{targets: []string{"fe-unit"}, condition: "matrix.shard <= 6"}),
			"make fe-checks":        exactWorkflowStep(6, "", workflowStepAuthority{targets: []string{"fe-checks"}, condition: "matrix.shard == 7"}),
			"make fe-tokens-verify": exactWorkflowStep(7, "Token artifacts are committed and current", workflowStepAuthority{targets: []string{"fe-tokens-verify"}, condition: "matrix.shard == 7"}),
			"make fe-packages":      exactWorkflowStep(8, "", workflowStepAuthority{targets: []string{"fe-packages"}, condition: "matrix.shard == 8"}),
		}),
		"ci-go-contracts.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			"make vet vet-tags lint": exactWorkflowStep(4, "Static analysis", workflowStepAuthority{targets: []string{"vet", "vet-tags", "lint"}, condition: "matrix.shard == 1"}),
			"make fmt privacy-verify tags-verify compose-verify go-race-verify eval-contract": exactWorkflowStep(5, "Repository contracts", workflowStepAuthority{targets: []string{"fmt", "privacy-verify", "tags-verify", "compose-verify", "go-race-verify", "eval-contract"}, condition: "matrix.shard == 2", environment: map[string]string{"MAKEFLAGS": "-j4 --output-sync=target"}}),
			"make go-shard-verify SHARDS=2": exactWorkflowStep(6, "The test lanes cover every package", workflowStepAuthority{targets: []string{"go-shard-verify"}, condition: "matrix.shard == 1"}),
			"make openapi-verify":           exactWorkflowStep(7, "OpenAPI spec is committed and current", workflowStepAuthority{targets: []string{"openapi-verify"}, condition: "matrix.shard == 1"}),
			"make retired-verify":           exactWorkflowStep(8, "Retired identifiers are gone from prose", workflowStepAuthority{targets: []string{"retired-verify"}, condition: "matrix.shard == 2"}),
			"make observability-verify":     exactWorkflowStep(9, "Observability artifacts are provisionable", workflowStepAuthority{targets: []string{"observability-verify"}, condition: "matrix.shard == 2", allowsAcquisition: true})}),
		"ci-go-cache-warm.yml": {
			environment: standardWorkflowEnvironment(),
			permissions: standardWorkflowPermissions(),
			jobs: map[string]workflowJobAuthority{
				"run": {steps: map[string]workflowStepAuthority{
					"make go-cache-warm GO_CACHE_WARM=${{ matrix.lane }}": exactWorkflowStep(4, "Compile the gate's test binaries without running them", workflowStepAuthority{targets: []string{"go-cache-warm"}, condition: "steps.warm.outputs.cache-hit != 'true'"}),
				}},
				"contracts": {steps: map[string]workflowStepAuthority{
					"make vet vet-tags lint": exactWorkflowStep(4, "Compile what the contracts analyse", workflowStepAuthority{targets: []string{"vet", "vet-tags", "lint"}, condition: "steps.warm.outputs.cache-hit != 'true'"}),
				}},
			},
		},
		"ci-go.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			"make test GO_TEST_LANE=${{ matrix.lane }}": exactWorkflowStep(11, "", workflowStepAuthority{targets: []string{"test"}}),
		}),
		"ci-image-certification.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			`IMAGE_CERT_REPORT="$RUNNER_TEMP/image-certification.json" make image-cert`: exactWorkflowStep(3, "Certify the release worker against the deterministic real-codec corpus", workflowStepAuthority{targets: []string{"image-cert"}}),
		}),
		"ci-image.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			packagedImageMetadataInspection: exactWorkflowStep(3, "Inspect packaged license and notice metadata", workflowStepAuthority{allowsAcquisition: true, shell: "bash"}),
			// #661: the unchanged FFmpeg gate against the binaries the image just built ships.
			shippedFFmpegExtraction: exactWorkflowStep(5, "Put the image's own ffmpeg and ffprobe on PATH", workflowStepAuthority{allowsAcquisition: true, shell: "bash"}),
			"make test-ffmpeg":      exactWorkflowStep(6, "FFmpeg acceptance against the shipped binaries", workflowStepAuthority{targets: []string{"test-ffmpeg"}}),
		}),
		"ci-playwright.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			"make fe-install":            exactWorkflowStep(4, "", workflowStepAuthority{targets: []string{"fe-install"}}),
			"make fe-codegen":            exactWorkflowStep(5, "", workflowStepAuthority{targets: []string{"fe-codegen"}}),
			playwrightVisualWorkflowStep: exactWorkflowStep(6, "Visual + a11y over storybook-static", workflowStepAuthority{targets: []string{"fe-visual"}, allowsAcquisition: true}),
			"make e2e":                   exactWorkflowStep(7, "Wizard e2e smoke vs a mocked backend", workflowStepAuthority{targets: []string{"e2e"}, allowsAcquisition: true, condition: "matrix.shard == 1"}),
		}),
		"ci-postgres.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			postgresTestCommand: exactWorkflowStep(4, "", workflowStepAuthority{targets: []string{"test-pg"}, allowsAcquisition: true}),
		}),
		"ci-rust-contracts.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			"make rust-check": exactWorkflowStep(2, "", workflowStepAuthority{targets: []string{"rust-check"}}),
		}),
		"ci-tuner.yml": {
			environment: standardWorkflowEnvironment(),
			// issues: read lets the flake quarantine read whether a tracking issue is still open.
			permissions: map[string]string{"contents": "read", "issues": "read"},
			jobs: map[string]workflowJobAuthority{
				"run": {steps: map[string]workflowStepAuthority{
					"make fe-install":            exactWorkflowStep(3, "", workflowStepAuthority{targets: []string{"fe-install"}}),
					"make tuner-quarantine":      exactWorkflowStep(5, "Resolve the flake quarantine", workflowStepAuthority{targets: []string{"tuner-quarantine"}, environment: map[string]string{"GH_TOKEN": "${{ github.token }}", "TUNER_QUARANTINE": "${{ inputs.quarantine }}"}}),
					"make tuner-e2e-host":        exactWorkflowStep(6, "100-Channel controller matrix", workflowStepAuthority{targets: []string{"tuner-e2e-host"}, environment: map[string]string{"TUNER_PROJECT": "${{ inputs.project }}", "TUNER_REPEAT_EACH": "${{ inputs.repeat_each }}", "TUNER_QUARANTINE": "${{ inputs.quarantine }}"}}),
					"make tuner-e2e-quarantine":  exactWorkflowStep(8, "Quarantined tests (non-blocking)", workflowStepAuthority{targets: []string{"tuner-e2e-quarantine"}, condition: "inputs.quarantine == 'exclude'", nonBlocking: true}),
					"make route-timing-e2e-host": exactWorkflowStep(10, "Route timing budgets (Chromium)", workflowStepAuthority{targets: []string{"route-timing-e2e-host"}}),
				}},
			},
		},
		"codeql.yml": codeQLWorkflowAuthority(),
		"ci.yml": {
			environment: standardWorkflowEnvironment(),
			permissions: standardWorkflowPermissions(),
			jobs: map[string]workflowJobAuthority{
				"changes": {steps: map[string]workflowStepAuthority{}},
				"ci-policy": {
					condition: "always()",
					steps: map[string]workflowStepAuthority{
						"make -j4 --output-sync=target ci-lint release-verify agent-harness-test shellcheck":        exactWorkflowStep(4, "Workflow, publication, agent harness, and shell policy", workflowStepAuthority{targets: []string{"ci-lint", "release-verify", "agent-harness-test", "shellcheck"}}),
						"go test ./docs && make arch-docs-verify config-docs-verify dev-docs-verify db-docs-verify": exactWorkflowStep(5, "Documentation contracts read by Go", workflowStepAuthority{targets: []string{"arch-docs-verify", "config-docs-verify", "dev-docs-verify", "db-docs-verify"}}),
					},
				},
				"ci-ok": {
					condition:   "always()",
					permissions: map[string]string{"actions": "read", "contents": "read", "issues": "write"},
					steps:       map[string]workflowStepAuthority{},
				},
			},
		},
		"merge-gate.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			"make vet": exactWorkflowStep(4, "The merged tree compiles, tests included", workflowStepAuthority{
				targets:   []string{"vet"},
				condition: "steps.go.outputs.changed == 'true'",
			}),
		}),
		"merge-gate-cache.yml": {
			environment: standardWorkflowEnvironment(),
			permissions: map[string]string{"actions": "write", "contents": "read"},
			jobs: map[string]workflowJobAuthority{
				"run": {steps: map[string]workflowStepAuthority{
					"make vet": exactWorkflowStep(4, "Compile what the gate compiles", workflowStepAuthority{
						targets:   []string{"vet"},
						condition: "steps.warm.outputs.cache-hit != 'true'",
					}),
					`./scripts/ci-merge-gate-cache-prune.sh "${{ steps.warm.outputs.cache-primary-key }}"`: exactWorkflowStep(6, "Delete older gate caches", workflowStepAuthority{
						condition:   "steps.warm.outputs.cache-hit != 'true'",
						environment: map[string]string{"GH_TOKEN": "${{ secrets.GITHUB_TOKEN }}", "REPO": "${{ github.repository }}"},
					}),
				}},
			},
		},
		"image-benchmark.yml": {
			permissions: standardWorkflowPermissions(),
			jobs: map[string]workflowJobAuthority{
				"benchmark": {
					steps: map[string]workflowStepAuthority{
						"make image-bench": exactWorkflowStep(3, "Measure complete AVIF ladders", workflowStepAuthority{
							targets: []string{"image-bench"},
							environment: map[string]string{
								"IMAGE_BENCH_RUNS":   "${{ inputs.runs }}",
								"IMAGE_BENCH_REPORT": "${{ runner.temp }}/image-benchmark-${{ matrix.platform }}.json",
							},
						}),
						"make image-parallelism-bench": exactWorkflowStep(4, "Compare AVIF process and thread shapes", workflowStepAuthority{
							targets: []string{"image-parallelism-bench"},
							environment: map[string]string{
								"IMAGE_BENCH_RUNS":         "${{ inputs.runs }}",
								"IMAGE_BENCH_ROLES":        "poster",
								"IMAGE_BENCH_CPU_PROFILES": "2,4",
								"IMAGE_BENCH_REPORT_DIR":   "${{ runner.temp }}/image-parallelism-${{ matrix.platform }}",
							},
						}),
					},
				},
			},
		},
		"ci-playout-bench.yml": standardRunWorkflow(map[string]workflowStepAuthority{
			"make playout-bench": exactWorkflowStep(6, "Bench the software pipeline", workflowStepAuthority{
				targets: []string{"playout-bench"},
				environment: map[string]string{
					"LOOMARR_ARTIFACT_DIR":       "${{ runner.temp }}/playout-bench",
					"PLAYOUT_BENCH_FAMILY":       "software",
					"PLAYOUT_BENCH_HEIGHT":       "720",
					"PLAYOUT_BENCH_START_RUNS":   "8",
					"PLAYOUT_BENCH_MAX_STREAMS":  "4",
					"PLAYOUT_BENCH_BASELINE_DIR": "docs/engineering/playout-bench/ci",
					"PLAYOUT_BENCH_TOLERANCE":    "0.4",
				},
			}),
		}),
		// The hardware bench is dispatch/schedule only and never a pull_request workflow (the repository
		// is public; see the header of playout-bench.yml and TestPlayoutBenchWorkflowNeverRunsOnPullRequest).
		"playout-bench.yml": {
			permissions: standardWorkflowPermissions(),
			jobs: map[string]workflowJobAuthority{
				"macos": {
					condition: playoutBenchMacOSGate,
					steps: map[string]workflowStepAuthority{
						playoutBenchRecordReport: playoutBenchRecordStep(5),
						"./scripts/playout-bench-macos-ffmpeg.sh \"$RUNNER_TEMP/loomarr-ffmpeg\"": exactWorkflowStep(2, "Install FFmpeg", workflowStepAuthority{allowsAcquisition: true}),
						"make playout-bench": exactWorkflowStep(3, "Bench the VideoToolbox pipeline", workflowStepAuthority{
							targets: []string{"playout-bench"},
							environment: map[string]string{
								"LOOMARR_ARTIFACT_DIR":       "${{ runner.temp }}/playout-bench",
								"PLAYOUT_BENCH_FAMILY":       "videotoolbox",
								"PLAYOUT_BENCH_THRESHOLDS":   "correctness",
								"PLAYOUT_BENCH_MAX_STREAMS":  "6",
								"PLAYOUT_BENCH_BASELINE_DIR": "docs/engineering/playout-bench/ci",
								"PLAYOUT_BENCH_TOLERANCE":    "0.4",
							},
						}),
					},
				},
				"gpu-t4": {
					condition: playoutBenchT4Gate,
					steps: map[string]workflowStepAuthority{
						playoutBenchRecordReport: playoutBenchRecordStep(8),
						"make playout-bench": exactWorkflowStep(6, "Bench the NVENC pipeline", workflowStepAuthority{
							targets: []string{"playout-bench"},
							environment: map[string]string{
								"LOOMARR_ARTIFACT_DIR":       "${{ runner.temp }}/playout-bench",
								"PLAYOUT_BENCH_FAMILY":       "nvenc",
								"PLAYOUT_BENCH_BASELINE_DIR": "docs/engineering/playout-bench/ci",
								"PLAYOUT_BENCH_TOLERANCE":    "0.4",
							},
						}),
					},
				},
				"arc": {
					condition: playoutBenchArcGate,
					steps: map[string]workflowStepAuthority{
						playoutBenchRecordReport: playoutBenchRecordStep(3),
						"make playout-bench": exactWorkflowStep(1, "Bench the VAAPI pipeline", workflowStepAuthority{
							targets: []string{"playout-bench"},
							environment: map[string]string{
								"LOOMARR_ARTIFACT_DIR": "${{ runner.temp }}/playout-bench",
								"PLAYOUT_BENCH_FAMILY": "vaapi",
								"PLAYOUT_BENCH_ACCEPT": "${{ inputs.accept && '1' || '' }}",
							},
						}),
					},
				},
				"geforce": {
					condition: playoutBenchGeForceGate,
					steps: map[string]workflowStepAuthority{
						playoutBenchRecordReport: playoutBenchRecordStep(3),
						"make playout-bench": exactWorkflowStep(1, "Bench the NVENC pipeline", workflowStepAuthority{
							targets: []string{"playout-bench"},
							environment: map[string]string{
								"LOOMARR_ARTIFACT_DIR": "${{ runner.temp }}/playout-bench",
								"PLAYOUT_BENCH_FAMILY": "nvenc",
								"PLAYOUT_BENCH_ACCEPT": "${{ inputs.accept && '1' || '' }}",
							},
						}),
					},
				},
				"coverage": {
					condition: "always()",
					steps: map[string]workflowStepAuthority{
						"./scripts/playout-bench-coverage.sh summarise": exactWorkflowStep(1, "Summarise which hardware legs ran", workflowStepAuthority{
							environment: map[string]string{
								"PLAYOUT_BENCH_NEEDS":          "${{ toJSON(needs) }}",
								"PLAYOUT_BENCH_EXPECT_MACOS":   "${{ " + playoutBenchMacOSGate + " }}",
								"PLAYOUT_BENCH_EXPECT_T4":      "${{ " + playoutBenchT4Gate + " }}",
								"PLAYOUT_BENCH_EXPECT_ARC":     "${{ " + playoutBenchArcGate + " }}",
								"PLAYOUT_BENCH_EXPECT_GEFORCE": "${{ " + playoutBenchGeForceGate + " }}",
							},
						}),
					},
				},
			},
		},
		"deadcode.yml": {
			environment: map[string]string{"GO_VERSION": "1.27"},
			permissions: map[string]string{"contents": "read"},
			jobs: map[string]workflowJobAuthority{
				"report": {
					permissions: map[string]string{"contents": "read", "issues": "write"},
					steps: map[string]workflowStepAuthority{
						"./scripts/deadcode.sh track docs/engineering/evidence/deadcode-baseline.txt": exactWorkflowStep(2, "Count unreachable functions and track growth", workflowStepAuthority{environment: map[string]string{
							"GH_TOKEN": "${{ secrets.GITHUB_TOKEN }}",
						}}),
					},
				},
			},
		},
		"pages.yml": {
			permissions: map[string]string{"contents": "read", "id-token": "write", "pages": "write"},
			jobs: map[string]workflowJobAuthority{
				"build":  {steps: map[string]workflowStepAuthority{}},
				"deploy": {steps: map[string]workflowStepAuthority{}},
			},
		},
		"release-notes.yml": {
			permissions: map[string]string{"contents": "write"},
			jobs: map[string]workflowJobAuthority{
				"publish-notes": {steps: map[string]workflowStepAuthority{}},
			},
		},
		"release.yml": {
			environment: map[string]string{"IMAGE": "ghcr.io/${{ github.repository }}", "REGISTRY": "ghcr.io"},
			permissions: map[string]string{"actions": "read", "contents": "read", "id-token": "write", "packages": "write"},
			jobs: map[string]workflowJobAuthority{
				"build":   {steps: map[string]workflowStepAuthority{}},
				"publish": {steps: map[string]workflowStepAuthority{}},
			},
		},
		"go-maintenance.yml": {
			environment: map[string]string{"GO_VERSION": "1.27"},
			permissions: map[string]string{"contents": "read"},
			jobs: map[string]workflowJobAuthority{
				"vulnerabilities": {steps: map[string]workflowStepAuthority{
					"make vuln": exactWorkflowStep(2, "Scan the code that ships", workflowStepAuthority{targets: []string{"vuln"}}),
				}},
				"library-scale": {steps: map[string]workflowStepAuthority{
					"make scan-scale": exactWorkflowStep(2, "Scan a 33k-file library", workflowStepAuthority{targets: []string{"scan-scale"}}),
				}},
			},
		},
		// The one workflow that publishes a release and opens a PR on a schedule. Everything it does is
		// in the script; the job's write permissions are exactly what the script's three side effects need.
		"ffmpeg-pin-refresh.yml": {
			environment: map[string]string{"GO_VERSION": "1.27"},
			permissions: map[string]string{"contents": "read"},
			jobs: map[string]workflowJobAuthority{
				"refresh": {
					permissions: map[string]string{"actions": "write", "contents": "write", "pull-requests": "write"},
					steps: map[string]workflowStepAuthority{
						`./scripts/ffmpeg-pin-refresh.sh automate "$RUNNER_TEMP/ffmpeg-pin"`: exactWorkflowStep(2, "Refresh the pin", workflowStepAuthority{
							allowsAcquisition: true,
							environment: map[string]string{
								"GH_TOKEN": "${{ github.token }}", "LOOMARR_REPO": "${{ github.repository }}",
								"DRY_RUN": "${{ inputs.dry_run || false }}",
							},
						}),
					},
				},
			},
		},
		// Read-only: the script asks archive.org and YouTube whether each curated source still exists.
		"curated-sources.yml": {
			permissions: map[string]string{"contents": "read"},
			jobs: map[string]workflowJobAuthority{
				"check": {steps: map[string]workflowStepAuthority{
					"./scripts/check-curated-sources.sh": exactWorkflowStep(1, "Check every entry against its provider", workflowStepAuthority{}),
				}},
			},
		},
		// Dispatch and schedule only: the cloud key must never reach a pull request.
		"staging.yml": {
			permissions: map[string]string{"contents": "read"},
			jobs: map[string]workflowJobAuthority{
				"build-image": {
					condition:   "github.event_name == 'workflow_dispatch' && inputs.action == 'up' && inputs.version == ''",
					permissions: map[string]string{"contents": "read", "packages": "write"},
					steps:       map[string]workflowStepAuthority{},
				},
				"control": {
					condition: "always() && (needs.build-image.result == 'success' || needs.build-image.result == 'skipped')",
					steps: map[string]workflowStepAuthority{
						`./scripts/staging.sh "$STAGING_ACTION"`: exactWorkflowStep(1, "Run the staging action", workflowStepAuthority{
							environment: map[string]string{
								"VULTR_API_KEY":       "${{ secrets.VULTR_API_KEY }}",
								"TS_AUTHKEY":          "${{ secrets.STAGING_TS_AUTHKEY }}",
								"CF_TUNNEL_TOKEN":     "${{ secrets.STAGING_CF_TUNNEL_TOKEN }}",
								"DEMO_ADMIN_PASSWORD": "${{ secrets.STAGING_DEMO_ADMIN_PASSWORD }}",
								"STAGING_ACTION":      "${{ inputs.action || 'sweep' }}",
								"STAGING_HOURS":       "${{ inputs.hours || '4' }}",
								"STAGING_DATABASE":    "${{ inputs.database || 'sqlite' }}",
								"STAGING_PUBLIC_URL":  "${{ inputs.public_url }}",
								"STAGING_GPU":         "${{ inputs.gpu || 'none' }}",
								"VERSION":             "${{ inputs.version }}",
								"STAGING_COMMIT":      "${{ inputs.action == 'up' && inputs.version == '' && github.sha || '' }}",
								"STAGING_IMAGE":       "${{ inputs.action == 'up' && inputs.version == '' && format('ghcr.io/{0}-staging:{1}', github.repository, github.sha) || '' }}",
							},
						}),
					},
				},
			},
		},
		"rust-maintenance.yml": {
			permissions: map[string]string{"contents": "read"},
			jobs: map[string]workflowJobAuthority{
				"supply-chain": {steps: map[string]workflowStepAuthority{}},
				"fuzz": {
					environment: map[string]string{"FUZZ_SECONDS": "${{ inputs.fuzz_seconds }}"},
					steps:       map[string]workflowStepAuthority{},
				},
			},
		},
		// Only the commit job may write contents; render and ci may only dispatch a workflow. None of
		// these jobs runs the branch's code: that happens in visual-baselines-render.yml, dispatched
		// at the branch (visual_baselines_workflow.go).
		"visual-baselines.yml": {
			permissions: map[string]string{"contents": "read"},
			jobs: map[string]workflowJobAuthority{
				"resolve": {
					permissions: map[string]string{"contents": "read", "pull-requests": "read"},
					steps: map[string]workflowStepAuthority{
						visualBaselinesResolveCommand + "\n": exactWorkflowStep(0, "Resolve and vet the pull request", workflowStepAuthority{shell: "bash", environment: map[string]string{
							"GH_TOKEN": "${{ github.token }}", "PR": "${{ inputs.pr }}", "REPO": "${{ github.repository }}",
							"REF": "${{ github.ref }}", "DEFAULT_BRANCH": "${{ github.event.repository.default_branch }}",
						}}),
					},
				},
				"render": {
					permissions: map[string]string{"actions": "write", "contents": "read"},
					steps: map[string]workflowStepAuthority{
						visualBaselinesRenderDispatchCommand + "\n": exactWorkflowStep(0, "Dispatch the render and wait for it", workflowStepAuthority{shell: "bash", environment: map[string]string{
							"GH_TOKEN": "${{ github.token }}", "REPO": "${{ github.repository }}",
							"BRANCH": "${{ needs.resolve.outputs.branch }}", "HEAD_SHA": "${{ needs.resolve.outputs.sha }}",
							"REQUEST": visualBaselinesRequest, "SERVER_URL": "${{ github.server_url }}",
							"FORCE": "${{ inputs.force }}",
						}}),
					},
				},
				"commit": {
					permissions: map[string]string{"actions": "read", "contents": "write"},
					steps: map[string]workflowStepAuthority{
						visualBaselinesVerifyRenderCommand + "\n": exactWorkflowStep(1, "Verify the render run", workflowStepAuthority{shell: "bash", environment: map[string]string{
							"GH_TOKEN": "${{ github.token }}", "REPO": "${{ github.repository }}",
							"BRANCH": "${{ needs.resolve.outputs.branch }}", "HEAD_SHA": "${{ needs.resolve.outputs.sha }}",
							"RENDER_RUN": "${{ needs.render.outputs.run_id }}", "REQUEST": visualBaselinesRequest,
						}}),
						visualBaselinesCommitCommand + "\n": exactWorkflowStep(3, "Commit and push only the changed baselines", workflowStepAuthority{shell: "bash", environment: map[string]string{
							"PR": "${{ inputs.pr }}", "BRANCH": "${{ needs.resolve.outputs.branch }}", "HEAD_SHA": "${{ needs.resolve.outputs.sha }}",
							"RUN_URL":     "${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}",
							"RUN_ATTEMPT": "${{ github.run_attempt }}",
							"RENDER_RUN":  "${{ needs.render.outputs.run_id }}",
							"FORCE":       "${{ inputs.force }}",
						}}),
					},
				},
				"ci": {
					permissions: map[string]string{"actions": "write", "contents": "read"},
					condition:   "needs.commit.outputs.pushed == 'true'",
					steps: map[string]workflowStepAuthority{
						visualBaselinesCIDispatchCommand + "\n": exactWorkflowStep(0, "Dispatch CI for the pushed commit", workflowStepAuthority{shell: "bash", environment: map[string]string{
							"GH_TOKEN": "${{ github.token }}", "REPO": "${{ github.repository }}",
							"BRANCH": "${{ needs.resolve.outputs.branch }}", "HEAD_SHA": "${{ needs.commit.outputs.sha }}",
						}}),
					},
				},
			},
		},
		// The one job that runs the branch's Make targets reads, and its first step refuses the
		// default branch's cache scope before any of the branch is fetched.
		"visual-baselines-render.yml": {
			environment: map[string]string{"NODE_VERSION": "22"},
			permissions: map[string]string{"contents": "read"},
			jobs: map[string]workflowJobAuthority{
				"render": {
					permissions: map[string]string{"contents": "read"},
					steps: map[string]workflowStepAuthority{
						visualBaselinesRenderGuardCommand + "\n": exactWorkflowStep(0, "Refuse the default branch", workflowStepAuthority{shell: "bash", environment: map[string]string{
							"REF": "${{ github.ref }}", "DEFAULT_BRANCH": "${{ github.event.repository.default_branch }}",
							"SHA": "${{ github.sha }}", "WANT_SHA": "${{ inputs.sha }}",
						}}),
						"make fe-install": exactWorkflowStep(5, "", workflowStepAuthority{targets: []string{"fe-install"}}),
						"make fe-codegen": exactWorkflowStep(6, "", workflowStepAuthority{targets: []string{"fe-codegen"}}),
						"make fe-visual-update": exactWorkflowStep(7, visualBaselinesUpdateStepName, workflowStepAuthority{
							targets: []string{"fe-visual-update"}, allowsAcquisition: true,
							environment: map[string]string{"LOOMARR_PLAYWRIGHT_FORCE": "${{ inputs.force }}"},
						}),
						visualBaselinesCollectCommand + "\n": exactWorkflowStep(8, "Collect the rewritten baselines", workflowStepAuthority{shell: "bash"}),
					},
				},
			},
		},
	}
}

// The hardware bench legs' gates. Each is both the leg's `if:` and its PLAYOUT_BENCH_EXPECT_* in the
// coverage job, so a gate cannot change on one side only and turn a skip back into silence (#1566).
const (
	playoutBenchMacOSGate    = "github.event_name == 'schedule' || inputs.run_macos"
	playoutBenchT4Gate       = "vars.PLAYOUT_BENCH_T4_RUNNER != '' && (github.event_name == 'schedule' || github.event_name == 'workflow_dispatch')"
	playoutBenchArcGate      = "github.event_name == 'workflow_dispatch' && inputs.run_arc"
	playoutBenchGeForceGate  = "github.event_name == 'workflow_dispatch' && inputs.run_geforce"
	playoutBenchRecordReport = `./scripts/playout-bench-coverage.sh record "$RUNNER_TEMP/playout-bench"`
)

// playoutBenchHardwareLegs is every hardware leg the coverage job must account for.
func playoutBenchHardwareLegs() []string { return []string{"macos", "gpu-t4", "arc", "geforce"} }

func playoutBenchRecordStep(index int) workflowStepAuthority {
	return exactWorkflowStep(index, "Record whether the bench reported", workflowStepAuthority{condition: "always()"})
}

func exactWorkflowStep(index int, name string, authority workflowStepAuthority) workflowStepAuthority {
	authority.exactName = true
	authority.name = name
	authority.exactStepIndex = true
	authority.stepIndex = index
	return authority
}

func codeQLWorkflowAuthority() workflowAuthority {
	analysisPermissions := map[string]string{"actions": "read", "contents": "read", "security-events": "write"}
	return workflowAuthority{
		permissions: standardWorkflowPermissions(),
		jobs: map[string]workflowJobAuthority{
			"changes": {steps: map[string]workflowStepAuthority{
				codeQLImpactCommand: exactWorkflowStep(1, "Select CodeQL languages", workflowStepAuthority{environment: map[string]string{
					"EVENT_NAME": "${{ github.event_name }}",
					"BASE":       "${{ github.event_name == 'pull_request' && github.event.pull_request.base.sha || github.event.before }}",
				}}),
			}},
			"analyze-actions":               {condition: "needs.changes.outputs.actions == 'true'", permissions: analysisPermissions},
			"analyze-go":                    {condition: "needs.changes.outputs.go == 'true'", permissions: analysisPermissions},
			"analyze-javascript-typescript": {condition: "needs.changes.outputs.javascript-typescript == 'true'", permissions: analysisPermissions},
			"analyze-python":                {condition: "needs.changes.outputs.python == 'true'", permissions: analysisPermissions},
			"analyze-ruby":                  {condition: "needs.changes.outputs.ruby == 'true'", permissions: analysisPermissions},
			"analyze-rust":                  {condition: "needs.changes.outputs.rust == 'true'", permissions: analysisPermissions},
		},
	}
}

func standardRunWorkflow(steps map[string]workflowStepAuthority) workflowAuthority {
	return workflowAuthority{
		environment: standardWorkflowEnvironment(),
		permissions: standardWorkflowPermissions(),
		jobs: map[string]workflowJobAuthority{
			"run": {steps: steps},
		},
	}
}
