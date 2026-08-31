package releaseverify

import (
	"fmt"
	"os"
	"strings"
)

var dockerfileCurlRequirements = []struct {
	option string
	value  string
}{
	{option: "--retry", value: "5"},
	{option: "--retry-all-errors"},
	{option: "--retry-delay", value: "2"},
	{option: "--retry-max-time", value: "600"},
}

// VerifyDockerfileDownloads ensures every active curl download uses the bounded
// retry policy required by release image builds.
func VerifyDockerfileDownloads(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Dockerfile: %w", err)
	}

	downloads := 0
	for _, instruction := range dockerfileInstructions(string(data)) {
		if !strings.HasPrefix(instruction, "RUN ") {
			continue
		}
		shell := strings.NewReplacer("&&", ";", "||", ";").Replace(strings.TrimPrefix(instruction, "RUN "))
		for _, statement := range strings.Split(shell, ";") {
			fields := activeShellFields(statement)
			if len(fields) == 0 || fields[0] != "curl" {
				continue
			}
			downloads++
			if err := verifyDockerfileCurl(fields); err != nil {
				return fmt.Errorf("dockerfile curl download %s: %w", curlOutput(fields), err)
			}
		}
	}
	if downloads == 0 {
		return fmt.Errorf("dockerfile has no active curl downloads")
	}
	return nil
}

func activeShellFields(statement string) []string {
	fields := strings.Fields(statement)
	for index, field := range fields {
		if strings.HasPrefix(field, "#") {
			return fields[:index]
		}
	}
	return fields
}

func verifyDockerfileCurl(command []string) error {
	for _, requirement := range dockerfileCurlRequirements {
		count := 0
		value := ""
		for index, field := range command {
			if strings.HasPrefix(field, requirement.option+"=") {
				return fmt.Errorf("option %s must use a separate value", requirement.option)
			}
			if field != requirement.option {
				continue
			}
			count++
			if requirement.value != "" {
				if index+1 >= len(command) {
					return fmt.Errorf("option %s has no value", requirement.option)
				}
				value = command[index+1]
			}
		}
		if count != 1 {
			return fmt.Errorf("requires exactly one %s option, found %d", requirement.option, count)
		}
		if requirement.value != "" && value != requirement.value {
			return fmt.Errorf("option %s is %q, want %q", requirement.option, value, requirement.value)
		}
	}
	return nil
}

func curlOutput(command []string) string {
	for index, field := range command {
		if field == "-o" && index+1 < len(command) {
			return command[index+1]
		}
	}
	return "with unknown output"
}
