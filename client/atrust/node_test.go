package atrust

import "testing"

func TestGetBestNodesSkipsMalformedAddresses(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("getBestNodes() panicked for malformed gateway node: %v", recovered)
		}
	}()

	got := getBestNodes(map[string][]string{
		"campus": {"malformed-node", "127.0.0.1:1"},
	})
	if got["campus"] != "127.0.0.1:1" {
		t.Fatalf("best node = %q, want first valid node", got["campus"])
	}
}
