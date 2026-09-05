package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/pangolin/pangolin/internal/benchmark"
	"github.com/pangolin/pangolin/internal/config"
	"github.com/pangolin/pangolin/internal/source"
	"github.com/pangolin/pangolin/internal/storage"
	"github.com/pangolin/pangolin/internal/subscription"
	"github.com/pangolin/pangolin/internal/version"
)

func Run(configPath string) error {
	if configPath == "" {
		configPath = "config.yaml"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	coreName := "mihomo"
	if runtime.GOOS == "windows" {
		coreName = "mihomo.exe"
	}
	corePath, err := filepath.Abs(filepath.Join("tools", "mihomo", coreName))
	if err != nil {
		return fmt.Errorf("无法解析 Mihomo 核心路径: %w", err)
	}
	if err := config.ValidateMihomo(corePath); err != nil {
		return fmt.Errorf("mihomo 核心不可用；请通过 GitHub Actions 下载官方核心: %w", err)
	}
	fetchCtx, cancelFetch := context.WithTimeout(context.Background(), time.Minute)
	defer cancelFetch()
	sources := make([]source.Source, len(cfg.Sources))
	for i, item := range cfg.Sources {
		sources[i] = source.Source{Name: item.Name, Primary: item.Primary, Fallbacks: item.Fallbacks}
	}
	results := source.FetchAll(fetchCtx, sources)
	merged := source.Merge(results)
	failedSources := 0
	failureDetails := make([]string, 0)
	for _, result := range results {
		if result.Err != nil {
			failedSources++
			failureDetails = append(failureDetails, result.Source+": "+result.Err.Error())
		}
	}
	nodes := make([]subscription.Proxy, 0, len(merged))
	for _, node := range merged {
		nodes = append(nodes, subscription.Proxy{Name: node.Name, Type: node.Type, Server: node.Server, Port: node.Port, Fields: node.Fields, Region: node.Region})
	}
	candidateCount := len(nodes)
	publishedCount := candidateCount
	if len(nodes) > 0 {
		proxyMaps := make([]map[string]any, 0, len(nodes))
		candidates := make([]benchmark.Candidate, 0, len(nodes))
		for _, node := range nodes {
			proxy := make(map[string]any, len(node.Fields)+4)
			for key, value := range node.Fields {
				proxy[key] = value
			}
			proxy["name"], proxy["type"], proxy["server"], proxy["port"] = node.Name, node.Type, node.Server, node.Port
			proxyMaps = append(proxyMaps, proxy)
			candidates = append(candidates, benchmark.Candidate{Name: node.Name})
		}
		probeConfig := benchmark.DefaultConfig()
		batchCount := (len(candidates) + 23) / 24
		probeBudget := batchCount * probeConfig.ProbeTimes * len(probeConfig.URLs)
		benchmarkTimeout := time.Duration(probeBudget)*probeConfig.ProbeTimeout + 2*time.Minute
		// Batches probe sequentially and a malformed node can force bisection
		// runs (-t) that add minutes; keep the floor above the worst case so
		// the context does not expire into a silent full-node drop.
		if benchmarkTimeout < 15*time.Minute {
			benchmarkTimeout = 15 * time.Minute
		}
		benchmarkCtx, cancelBenchmark := context.WithTimeout(context.Background(), benchmarkTimeout)
		defer cancelBenchmark()
		metrics, dropped, err := benchmark.Benchmark(benchmarkCtx, corePath, proxyMaps, candidates, probeConfig)
		if err != nil {
			io.WriteString(os.Stdout, "benchmark 不可用（"+err.Error()+"），跳过质量探测\n")
		}
		if len(dropped) > 0 {
			io.WriteString(os.Stdout, fmt.Sprintf("丢弃 %d 个 Mihomo 不接受配置的节点\n", len(dropped)))
		}
		published := make([]subscription.Proxy, 0, len(nodes))
		metricByName := make(map[string]benchmark.Metric, len(metrics))
		for _, metric := range metrics {
			metricByName[metric.Name] = metric
		}
		for _, node := range nodes {
			if metric, ok := metricByName[node.Name]; ok && benchmark.Eligible(metric, probeConfig) {
				published = append(published, node)
			}
		}
		sort.SliceStable(published, func(i, j int) bool {
			a, b := metricByName[published[i].Name], metricByName[published[j].Name]
			if a.LatencyMs != b.LatencyMs {
				return a.LatencyMs < b.LatencyMs
			}
			if a.JitterMs != b.JitterMs {
				return a.JitterMs < b.JitterMs
			}
			return a.HealthScore > b.HealthScore
		})
		nodes = published
		publishedCount = len(nodes)
	}
	out, err := subscription.Marshal(subscription.Build(nodes))
	if err != nil {
		return fmt.Errorf("生成订阅失败: %w", err)
	}
	if err := benchmark.ValidateConfig(out, corePath); err != nil {
		return fmt.Errorf("订阅未通过 Mihomo 校验，拒绝发布: %w", err)
	}
	outputPath := "clash.yaml"
	if err := storage.AtomicWrite(outputPath, out, 0600); err != nil {
		return fmt.Errorf("写入订阅失败: %w", err)
	}
	regions := map[string]bool{}
	for _, node := range nodes {
		if node.Region != "" && node.Region != "OTHER" {
			regions[node.Region] = true
		}
	}
	message := "已生成订阅"
	if len(nodes) == 0 {
		message += "（当前无可用节点，使用 DIRECT-FALLBACK）"
	} else {
		message += fmt.Sprintf("（已接入 %d 个节点）", len(nodes))
	}
	io.WriteString(os.Stdout, fmt.Sprintf("Pangolin %s %s\n源: %d（失败 %d）\n候选: %d，发布: %d，区域: %d\n输出: %s\n", version.Version, message, len(sources), failedSources, candidateCount, publishedCount, len(regions), outputPath))
	if len(failureDetails) > 0 {
		io.WriteString(os.Stdout, "源失败详情: "+strings.Join(failureDetails, "; ")+"\n")
	}
	return nil
}
