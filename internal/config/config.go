package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Sources []Source `yaml:"sources"`
}
type Source struct {
	Name      string   `yaml:"name"`
	Primary   string   `yaml:"primary"`
	Fallbacks []string `yaml:"fallbacks"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read configuration file %q: %w", path, err)
	}
	var cfg Config
	if err := parse(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse configuration file %q: %w", path, err)
	}
	if err := cfg.Validate(path); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate(path string) error {
	if len(c.Sources) == 0 {
		return fmt.Errorf("configuration file %q has no sources; add at least one subscription source", path)
	}
	for i, source := range c.Sources {
		if strings.TrimSpace(source.Name) == "" {
			return fmt.Errorf("configuration file %q has an empty sources[%d].name; provide a source name", path, i)
		}
		if err := validateURL(source.Primary); err != nil {
			return fmt.Errorf("configuration file %q has an invalid sources[%d].primary: %w", path, i, err)
		}
		for j, fallback := range source.Fallbacks {
			if err := validateURL(fallback); err != nil {
				return fmt.Errorf("configuration file %q has an invalid sources[%d].fallbacks[%d]: %w", path, i, j, err)
			}
		}
	}
	return nil
}

func parse(data []byte, cfg *Config) error {
	return yaml.Unmarshal(data, cfg)
}

func validateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" {
		return errors.New("must be an HTTP or HTTPS URL with a hostname")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return errors.New("must not target a local hostname")
	}
	if ip := net.ParseIP(host); ip != nil && isPrivateIP(ip) {
		return errors.New("must not target a private or local IP address")
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func ValidateMihomo(path string) error {
	info, err := os.Stat(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("mihomo file %q does not exist or is not accessible: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("mihomo path %q points to a directory; provide an executable file path", path)
	}
	if runtime.GOOS == "windows" && strings.ToLower(filepath.Ext(path)) != ".exe" {
		return fmt.Errorf("mihomo file %q is not an .exe file; provide a Windows executable", path)
	}
	if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
		return fmt.Errorf("mihomo file %q is not executable; run chmod +x", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("mihomo file %q cannot be read: %w", path, err)
	}
	_ = f.Close()
	return nil
}
