package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTrustedProxies(t *testing.T) {
	dir := t.TempDir()

	write := func(name, contents string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	t.Run("parses a list of proxies", func(t *testing.T) {
		path := write("settings.toml", "trusted_proxies=[\"127.0.0.1/32\", \"::1/128\"]\n")
		conf := Load(path)
		want := []string{"127.0.0.1/32", "::1/128"}
		if len(conf.TrustedProxies) != len(want) {
			t.Fatalf("TrustedProxies = %#v, want %#v", conf.TrustedProxies, want)
		}
		for i := range want {
			if conf.TrustedProxies[i] != want[i] {
				t.Fatalf("TrustedProxies = %#v, want %#v", conf.TrustedProxies, want)
			}
		}
	})

	t.Run("defaults to trusting nobody", func(t *testing.T) {
		path := write("settings-default.toml", "listen_port=8989\n")
		conf := Load(path)
		if len(conf.TrustedProxies) != 0 {
			t.Fatalf("TrustedProxies = %#v, want empty", conf.TrustedProxies)
		}
	})
}
