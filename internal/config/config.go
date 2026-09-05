package config

import (
	"errors"
	"fmt"
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
		return Config{}, fmt.Errorf("读取配置文件 %q 失败: %w", path, err)
	}
	var cfg Config
	if err := parse(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("解析配置文件 %q 失败: %w", path, err)
	}
	if err := cfg.Validate(path); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate(path string) error {
	if len(c.Sources) == 0 {
		return fmt.Errorf("配置文件 %q 的 sources 为空，请至少添加一个订阅源", path)
	}
	for i, source := range c.Sources {
		if strings.TrimSpace(source.Name) == "" {
			return fmt.Errorf("配置文件 %q 的 sources[%d].name 为空，请填写源名称", path, i)
		}
		if err := validateURL(source.Primary); err != nil {
			return fmt.Errorf("配置文件 %q 的 sources[%d].primary 无效: %w", path, i, err)
		}
		for j, fallback := range source.Fallbacks {
			if err := validateURL(fallback); err != nil {
				return fmt.Errorf("配置文件 %q 的 sources[%d].fallbacks[%d] 无效: %w", path, i, j, err)
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
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("必须是包含主机名的 HTTP/HTTPS URL")
	}
	return nil
}

func ValidateMihomo(path string) error {
	info, err := os.Stat(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("mihomo 文件 %q 不存在或不可访问: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("mihomo 路径 %q 指向目录，请填写可执行文件路径", path)
	}
	if runtime.GOOS == "windows" && strings.ToLower(filepath.Ext(path)) != ".exe" {
		return fmt.Errorf("mihomo 文件 %q 不是 .exe 文件，请填写 Windows 可执行文件", path)
	}
	if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
		return fmt.Errorf("mihomo 文件 %q 没有执行权限，请使用 chmod +x", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("mihomo 文件 %q 无法读取: %w", path, err)
	}
	_ = f.Close()
	return nil
}
