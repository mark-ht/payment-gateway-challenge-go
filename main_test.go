package main

import "testing"

func TestResolveBuildMetadata(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want buildMetadata
	}{
		{
			name: "defaults unset values",
			want: buildMetadata{version: "dev", commit: "none", date: "unknown"},
		},
		{
			name: "defaults empty values",
			env: map[string]string{
				"APP_VERSION": "",
				"APP_COMMIT":  "",
				"APP_DATE":    "",
			},
			want: buildMetadata{version: "dev", commit: "none", date: "unknown"},
		},
		{
			name: "uses injected values",
			env: map[string]string{
				"APP_VERSION": "v1.2.3",
				"APP_COMMIT":  "abc123",
				"APP_DATE":    "2026-04-02T00:00:00Z",
			},
			want: buildMetadata{version: "v1.2.3", commit: "abc123", date: "2026-04-02T00:00:00Z"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveBuildMetadata(func(key string) string {
				return test.env[key]
			})
			if got != test.want {
				t.Errorf("resolveBuildMetadata() = %#v, want %#v", got, test.want)
			}
		})
	}
}
