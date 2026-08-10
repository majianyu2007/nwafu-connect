package hook_func

import (
	"reflect"
	"testing"
)

func TestParseDNSServers(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{name: "none", output: "There aren't any DNS Servers set on Wi-Fi.\n"},
		{name: "empty", output: "\n"},
		{name: "servers", output: "  1.1.1.1\n2606:4700:4700::1111\n", want: []string{"1.1.1.1", "2606:4700:4700::1111"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := parseDNSServers(test.output); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseDNSServers(%q) = %#v, want %#v", test.output, got, test.want)
			}
		})
	}
}
