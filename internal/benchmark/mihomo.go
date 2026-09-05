package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxLogBytes = 64 << 10

type Mihomo struct {
	CoreDir    string
	ConfigPath string
	API        Controller
	cmd        *exec.Cmd
	log        *limitedBuffer
}

func NewMihomo(ctx context.Context, core string, config []byte, geoIPPath string) (*Mihomo, error) {
	if strings.TrimSpace(core) == "" {
		return nil, errors.New("mihomo 路径为空")
	}
	dir, err := os.MkdirTemp("", "pangolin-benchmark-")
	if err != nil {
		return nil, fmt.Errorf("创建 benchmark 临时目录失败: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(dir)
		}
	}()
	configPath := filepath.Join(dir, "config.yaml")
	controlPort, err := freePort()
	if err != nil {
		return nil, err
	}
	// freePort under the same 127.0.0.1 can hand out the same port twice;
	// binding external-controller and mixed-port to one port makes Mihomo
	// fail to start with an address-in-use conflict.
	mixedPort, err := freePort()
	if err != nil {
		return nil, err
	}
	for mixedPort == controlPort {
		mixedPort, err = freePort()
		if err != nil {
			return nil, err
		}
	}
	config, err = withPorts(config, controlPort, mixedPort)
	if err != nil {
		return nil, fmt.Errorf("写入 benchmark 端口配置失败: %w", err)
	}
	if err := os.WriteFile(configPath, config, 0600); err != nil {
		return nil, fmt.Errorf("写入 benchmark 配置失败: %w", err)
	}
	if geoIPPath != "" {
		data, err := os.ReadFile(geoIPPath)
		if err != nil {
			return nil, fmt.Errorf("读取 GeoIP 数据库失败: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "geoip.metadb"), data, 0600); err != nil {
			return nil, fmt.Errorf("复制 GeoIP 数据库失败: %w", err)
		}
	}
	if err := checkConfig(ctx, core, configPath); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, core, "-d", dir, "-f", configPath)
	cmd.Dir = dir
	logs := &limitedBuffer{limit: maxLogBytes}
	cmd.Stdout = logs
	cmd.Stderr = logs
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 Mihomo 失败: %w", err)
	}
	m := &Mihomo{CoreDir: dir, ConfigPath: configPath, API: Controller{BaseURL: fmt.Sprintf("http://127.0.0.1:%d", controlPort), Client: &http.Client{Timeout: controllerClientTimeout}}, cmd: cmd, log: logs}
	keep = true
	if err := m.waitReady(ctx); err != nil {
		_ = m.Close()
		return nil, err
	}
	return m, nil
}

// controllerClientTimeout is the http.Client deadline for controller calls.
// It must exceed the per-URL probe budget (DefaultConfig.ProbeTimeout) by a
// margin so Mihomo's own delay budget, not the client's deadline, decides a
// failed test; the 2s value used here cut reachable measurements roughly in
// half at 24-way concurrency on the reference node pool.
const controllerClientTimeout = 3 * time.Second

// ValidateConfig runs `mihomo -t -f` against the given config so a generated
// subscription is rejected before being published if Mihomo does not accept it.
func ValidateConfig(config []byte, core string) error {
	if strings.TrimSpace(core) == "" {
		return errors.New("mihomo 路径为空")
	}
	dir, err := os.MkdirTemp("", "pangolin-validate-")
	if err != nil {
		return fmt.Errorf("创建校验临时目录失败: %w", err)
	}
	defer os.RemoveAll(dir)
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, config, 0600); err != nil {
		return fmt.Errorf("写入校验配置失败: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return checkConfig(ctx, core, configPath)
}

// ConfigRejectedError marks a failed `mihomo -t` so batch benchmarking can
// bisect malformed candidates instead of treating engine failures as config
// problems (or vice versa).
type ConfigRejectedError struct{ Err error }

func (e *ConfigRejectedError) Error() string { return e.Err.Error() }
func (e *ConfigRejectedError) Unwrap() error { return e.Err }

// checkConfig runs `mihomo -t -f configPath` from the config's directory.
func checkConfig(ctx context.Context, core, configPath string) error {
	check := exec.CommandContext(ctx, core, "-t", "-f", configPath)
	check.Dir = filepath.Dir(configPath)
	if output, err := check.CombinedOutput(); err != nil {
		return &ConfigRejectedError{Err: fmt.Errorf("mihomo 配置校验失败: %w: %s", err, summarize(output))}
	}
	return nil
}

func (m *Mihomo) waitReady(ctx context.Context) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("mihomo controller 启动超时: %s", m.Log())
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(m.API.BaseURL, "/")+"/version", nil)
			resp, err := m.API.Client.Do(req)
			if err != nil {
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
	}
}

func (m *Mihomo) Close() error {
	if m == nil {
		return nil
	}
	var err error
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
		_, err = m.cmd.Process.Wait()
	}
	if m.CoreDir != "" {
		if cleanErr := os.RemoveAll(m.CoreDir); err == nil {
			err = cleanErr
		}
	}
	return err
}
func (m *Mihomo) Log() string {
	if m == nil || m.log == nil {
		return ""
	}
	return m.log.String()
}

func withPorts(data []byte, controlPort, mixedPort int) ([]byte, error) {
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析 benchmark 配置失败: %w", err)
	}
	config["external-controller"] = fmt.Sprintf("127.0.0.1:%d", controlPort)
	config["mixed-port"] = mixedPort
	config["allow-lan"] = false
	return json.Marshal(config)
}
func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("分配 localhost 端口失败: %w", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
func summarize(data []byte) string {
	text := strings.TrimSpace(string(data))
	if len(text) > 2000 {
		return text[:2000] + "..."
	}
	return text
}

type limitedBuffer struct {
	data  []byte
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - len(b.data)
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.data = append(b.data, p...)
	}
	return len(p), nil
}
func (b *limitedBuffer) String() string { return string(b.data) }

var _ io.Writer = (*limitedBuffer)(nil)
