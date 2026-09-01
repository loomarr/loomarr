package releaseverify

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The private values removed during the first-beta fixture audit never belong in source,
// including here. Their case-folded SHA-256 fingerprints retain the exact regression check
// without retaining or printing a usable value. The final two fingerprints are non-private
// sentinels that let tests exercise the same production interface and fixture-only scope.
var privateFixtureFingerprints = map[string]string{
	"425997118cfa504677beaa951a1218d8c17d55b9429cf9bafb5a884cf8338c86": "private Live TV address",
	"8b51baea68b18710dd1d3ffbd6c84816e527cb485baeb58ff4252fdc5630deac": "private tuner address",
	"7ab56f45c3f95a0a428f5e212f6c4026264cca2f088727bf9da59a72bd521e8d": "private media-server address",
	"8aaaba99ee3a238b0eeccf0b3011d8b1937fe03a21ada49ef3d7eb3cc12eb001": "private app address",
	"8d9d7f390ea02b10f2a03144fe473353ad7c7d62f490fbdf845d5c10f3494261": "live media-server hostname",
	"6a994e8e0c22937d7d0dd67072119cbd72effa8b103c89b2d46e74452614ec8e": "captured personal email",
	"8d7fdc3890c51640f2128f256dc908571f804209a8db5c006095bfa4e6a4f4dc": "captured personal email",
	"aa772ab6afdf0a02770c480967ea5b8a904d33ca4ec252e376b894a3d2582782": "captured personal email",
	"c8e04438762f6a955618e7db16aa53da449e837192043d73614fb398caa0ed5d": "captured personal email",
	"fb613559f9b231237dfaea0a5939f9f0fadfa0d6e7b3346cc57b4e309b5615c8": "captured personal email",
	"0edadbb013fd8453a996ca26e9f92f8f5d994506445a87e831a37024ea732e6b": "captured server id",
	"06f11cb3c2beec39ad0a9e46a5f9e87d7cdf8a4ce95534a1bf581151708f87db": "captured user id",
	"78e8041a3a3752e2b4da00b6f83ceb0640567fb214b69a8fbf4f425aa1eefb5f": "captured user id",
	"e786b998d323c9c7ef2965f1717187ead6757a535a63e766df6ba1a15ea981b8": "captured user id",
	"50a577f13d741d537378a24ef80a4f7095e9a89fff6143ce36f1117eebc975b6": "captured user id",
	"22133196bf1f3ff3d29de6459bbb22d703181d8e8a52f65d2c621d996d1045d1": "captured user id",
	"8caaf9856067b742d1961c7b9aa949770844e01da60a2f8e3f22a85e7e18cf68": "captured user id",
	"8e7921d507bfc7e7010fd19b593f00b22d34092bce5ae52eb22c4fa19fe1d68a": "captured user id",
	"322766a25bb4b8a41ca2db239c2f8ef9b2754a88b7358612d7dfb01731647fad": "captured user id",
	"f9c9f50ba97edb1411b46dfdd4579b0617c43c5d9ef1d2c6ad7048113e996cd3": "captured user id",
	"d870787f780f9677284903ff6ca105138521b27599d71dda25ff307ccb03f8c8": "captured user id",
	"37a5a659d18cad9b5b22833a2a0750d602ddfc25773b7b8ff3d07eb3aeb0fdf6": "captured tuner id",
	"f28aa2bcef2dc219ec3064f6d221ebc81406e614956f45cb80e811f94f3b6cb6": "captured listing id",
	"58bfb7b2f7fffd4a380ff416b212b68cca6aca5a9fccc6ee964ccca53f834487": "captured tuner id",
	"632efcadd239c2404d502785c14f781e83decb35e3ed51dbf125c22bf71ae63b": "captured hardware address",
	"219ddd11b35b4753407522cc32713e8c8d0842e27c269ae1e459d53c39eb0062": "captured hardware address",
	"d15489fdc3b21f4f419e58947481d57dde70440d270294c2e039ee3fc76c2d6b": "captured hardware address",
	"911f3d3ea12da90601fe05689e13d712551f1c2fe175eae213eb01b6be896b06": "captured household name",
	"6d219eef9ed1213b994241c031488503c3843b8dc91bc26aedd24b70228e1293": "captured household name",
	"e010a14d62b51da432ee72399a6ba09070a5ae47bd1f5bfd18e8d586f0aa813a": "captured household name",
	"444a131817a4c64fd0f851bfa0067b92eba75a003d37a62106c6cb1af8a1e527": "captured household name",
	"26e1400d63222defb5c64de18a88a3ad166683982ee36ac11171f4c87627ac45": "captured household name",
	"89856140ce2e0cdcdcf0f0969dcfa32cf912403707a2c8099e0961c9d37c7d9f": "captured household name",
	"74eaf15c4394e49604de0c3c026617b1c9cc5926f1eb1308fea3b8dfed80433d": "captured household name",
	"1567dae7f504ee2c440d992e6b2a180eec3c902cdb326d50a19e5211a1662645": "captured household name",
	"a49169515985dcb3fb79324fe794e0b8cd871c7083641064cea364556a0be1c7": "captured household name",
	"4c9ab350808e40e341c98c33b12a088b2e9f0c276918bc86232c6c21c089ffa0": "captured household name",
	"f5b0edbf69c1c6caf8be0ac084598aa3b6aec96e1573291699212190851ac6d1": "captured household name",
	"9eb1d8d316b63e83a9ad4b72a1d277b7cc0fa66483ba8cc3af2a77e9b585681f": "captured account name",
	"75ef0a3922ff6e59f3a9a9981f56c07e4e1a6f897647c2c9f80ad222d572839c": "captured account name",
	"dd26ef44f54fcea3918bc856801f616ad5854104923f5ba9b8da528dc5143c1d": "captured profile PIN",
	"3c20e4ea1540e8a9e4f3792764b6b1e88c3cb053dbed9a8142ec69f24e8e5321": "captured profile PIN",
	"9a30575229a11f77aab25c846c01fe7c6fcab1679be3c87078d213f9c660917f": "private-fixture regression sentinel",
	"268dad736496c95f7eb7ba9f2aae5ce9c2136359567db100a9155ffd457d991e": "private-fixture field regression sentinel",
}

var privateFixtureCandidatePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:[0-9]{1,3}\.){3}[0-9]{1,3}`),
	regexp.MustCompile(`[[:alnum:]._%+\-]+@[[:alnum:].\-]+\.[[:alpha:]]{2,}`),
	regexp.MustCompile(`[[:alnum:]_-]+(?:\.[[:alnum:]_-]+){2,}`),
	regexp.MustCompile(`[[:xdigit:]]{32}`),
	regexp.MustCompile(`[[:xdigit:]]{12}`),
}

var privateFixtureFieldPattern = regexp.MustCompile(`"(?:Name|jellyfinUsername|displayName|ProfilePin)"[[:space:]]*:[[:space:]]*"[^"]*"`)

var privateFixtureFieldPaths = map[string]struct{}{
	"internal/testkit/fixtures/emby/auth_success_response.json":  {},
	"internal/testkit/fixtures/emby/users_list.json":             {},
	"internal/testkit/fixtures/seerr/request_available_201.json": {},
	"internal/testkit/fixtures/seerr/request_repeat.json":        {},
}

// VerifyPrivateFixtures rejects a tracked candidate whose case-folded SHA-256 fingerprint
// matches one removed during the capture audit. It reports only labels and fingerprints.
func VerifyPrivateFixtures(root string) error {
	paths, err := trackedFiles(root)
	if err != nil {
		return err
	}

	seen := make(map[string]struct{})
	violations := make(map[string]struct{})
	for _, relative := range paths {
		path := filepath.Join(root, filepath.FromSlash(relative))
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("private-fixture guard: inspect tracked file %s: %w", relative, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("private-fixture guard: read tracked file %s: %w", relative, err)
		}
		if bytes.IndexByte(data, 0) >= 0 {
			continue
		}
		for _, pattern := range privateFixtureCandidatePatterns {
			checkPrivateFixtureMatches(pattern.FindAll(data, -1), seen, violations)
		}
		if _, auditedFixture := privateFixtureFieldPaths[filepath.ToSlash(relative)]; auditedFixture {
			checkPrivateFixtureMatches(privateFixtureFieldPattern.FindAll(data, -1), seen, violations)
		}
	}
	if len(violations) == 0 {
		return nil
	}

	messages := make([]string, 0, len(violations))
	for violation := range violations {
		messages = append(messages, violation)
	}
	sort.Strings(messages)
	return errors.New(strings.Join(messages, "; "))
}

func trackedFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z", "--cached")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("private-fixture guard: enumerate tracked files: %w", err)
	}
	trimmed := bytes.TrimSuffix(output, []byte{0})
	if len(trimmed) == 0 {
		return nil, nil
	}
	parts := bytes.Split(trimmed, []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		paths = append(paths, string(part))
	}
	return paths, nil
}

func checkPrivateFixtureMatches(matches [][]byte, seen, violations map[string]struct{}) {
	for _, match := range matches {
		candidate := strings.ToLower(string(match))
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		sum := sha256.Sum256([]byte(candidate))
		digest := hex.EncodeToString(sum[:])
		if label, found := privateFixtureFingerprints[digest]; found {
			violations[fmt.Sprintf("private-fixture guard: %s fingerprint remains (%s)", label, digest)] = struct{}{}
		}
	}
}
