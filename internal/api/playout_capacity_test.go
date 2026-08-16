package api

import (
	"strings"
	"testing"
)

func TestAtCapacityDetailOffersSafeRecovery(t *testing.T) {
	if !strings.Contains(atCapacityDetail, "wait") || !strings.Contains(atCapacityDetail, "lower quality") {
		t.Errorf("at-capacity detail does not offer safe recovery choices: %q", atCapacityDetail)
	}
	if strings.Contains(atCapacityDetail, "Raise") {
		t.Errorf("at-capacity detail promises capacity can be raised past measurement: %q", atCapacityDetail)
	}
}
