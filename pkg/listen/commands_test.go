package listen

import "testing"

func TestParseProviderCommand(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantCmd string
		wantArg string
	}{
		{name: "provider with argument", text: "/provider openai", wantCmd: "/provider", wantArg: "openai"},
		{name: "provider with bot suffix", text: "/provider@cccbot zai", wantCmd: "/provider", wantArg: "zai"},
		{name: "providers list", text: "/providers", wantCmd: "/providers", wantArg: ""},
		{name: "providers with bot suffix", text: "/providers@cccbot", wantCmd: "/providers", wantArg: ""},
		{name: "providers ignores trailing text", text: "/providers list", wantCmd: "/providers", wantArg: "list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCmd, gotArg := parseProviderCommand(tt.text)
			if gotCmd != tt.wantCmd || gotArg != tt.wantArg {
				t.Fatalf("parseProviderCommand(%q) = (%q, %q), want (%q, %q)", tt.text, gotCmd, gotArg, tt.wantCmd, tt.wantArg)
			}
		})
	}
}
