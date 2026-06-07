package config

import "testing"

func TestResolveAdminListen(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		listen   string
		wantHost string
		wantPort int
	}{
		{"all interfaces explicit", "0.0.0.0:9090", "0.0.0.0", 9090},
		{"port only colon form", ":9090", "0.0.0.0", 9090},
		{"port only digits", "9090", "0.0.0.0", 9090},
		{"localhost explicit", "127.0.0.1:9090", "127.0.0.1", 9090},
		{"default when empty", "", "127.0.0.1", 9090},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &FileConfig{
				Admin: &AdminConfig{
					Enabled: true,
					Listen:  tc.listen,
				},
			}
			got, err := ResolveAdmin(cfg, "/tmp/server.yaml")
			if err != nil {
				t.Fatalf("ResolveAdmin: %v", err)
			}
			if got.Host != tc.wantHost || got.Port != tc.wantPort {
				t.Fatalf("listen=%q => host=%q port=%d, want host=%q port=%d",
					tc.listen, got.Host, got.Port, tc.wantHost, tc.wantPort)
			}
		})
	}
}
