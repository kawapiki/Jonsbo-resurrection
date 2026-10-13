package aiadapters

import (
	"testing"

	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
)

// Native Codex can lose its in-memory account after a forced OAuth refresh even
// while the existing access token still serves quota and token-usage requests.
func TestOpenAIPollCollectsUsageWithoutForcingNativeTokenRenewal(t *testing.T) {
	m, runner, observations := testNativeManager(t)
	defer m.cancel()
	defer func() {
		if m.rpc != nil {
			m.rpc.Close()
		}
	}()
	m.enabled[data.OpenAI] = true
	runner.forcedRefreshClearsAccount = true

	for poll := 0; poll < 2; poll++ {
		before := len(*observations)
		if err := m.refresh(data.OpenAI); err != nil {
			t.Fatalf("poll %d discarded a usable native sign-in: %v", poll, err)
		}
		kinds := map[string]bool{}
		for _, observation := range (*observations)[before:] {
			kinds[observation.Kind] = true
		}
		if !kinds["account"] || !kinds["quota"] || !kinds["usage"] {
			t.Fatalf("poll %d did not collect account, quota and usage: %v", poll, kinds)
		}
		if status := providerJSON(t, m, data.OpenAI); status["login"] != "signed-in" || status["connection"] != "connected" {
			t.Fatalf("poll %d falsely requested sign-in: %v", poll, status)
		}
	}
}
