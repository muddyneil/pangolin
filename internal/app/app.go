package app

import (
	"context"
	"errors"
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
		return fmt.Errorf("failed to resolve Mihomo core path: %w", err)
	}
	if err := config.ValidateMihomo(corePath); err != nil {
		return fmt.Errorf("Mihomo core is unavailable; download the official core through GitHub Actions: %w", err)
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
	if failedSources == len(sources) {
		return fmt.Errorf("all %d subscription sources failed; existing output was preserved", len(sources))
	}
	if len(merged) == 0 {
		return fmt.Errorf("no usable proxy nodes were fetched; existing output was preserved")
	}
	publicNodes := make([]source.Proxy, 0, len(merged))
	serverCtx, cancelServerCheck := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelServerCheck()
	for _, node := range merged {
		if err := source.PublicHost(serverCtx, node.Server); err != nil {
			io.WriteString(os.Stdout, fmt.Sprintf("Dropped node %q: %s\n", node.Name, err))
			continue
		}
		publicNodes = append(publicNodes, node)
	}
	merged = publicNodes
	if len(merged) == 0 {
		return fmt.Errorf("no proxy nodes with public server addresses were fetched; existing output was preserved")
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
		benchmarkTimeout := benchmark.EstimatedTimeout(len(candidates), probeConfig)
		benchmarkCtx, cancelBenchmark := context.WithTimeout(context.Background(), benchmarkTimeout)
		defer cancelBenchmark()
		metrics, dropped, err := benchmark.Benchmark(benchmarkCtx, corePath, proxyMaps, candidates, probeConfig)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("node quality probe exceeded its %s budget and was cancelled; existing output was preserved: %w", benchmarkTimeout, err)
			}
			return fmt.Errorf("node quality probe failed; existing output was preserved: %w", err)
		}
		if len(metrics)+len(dropped) != len(nodes) {
			return fmt.Errorf("node quality probe accounted for %d of %d candidates; existing output was preserved", len(metrics)+len(dropped), len(nodes))
		}
		if len(dropped) > 0 {
			io.WriteString(os.Stdout, fmt.Sprintf("Dropped %d nodes rejected by Mihomo configuration: %s\n", len(dropped), strings.Join(dropped, ", ")))
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
		if publishedCount == 0 {
			return fmt.Errorf("no proxy nodes passed quality checks; existing output was preserved")
		}
	}
	out, err := subscription.Marshal(subscription.Build(nodes))
	if err != nil {
		return fmt.Errorf("failed to generate subscription: %w", err)
	}
	if err := benchmark.ValidateConfig(out, corePath); err != nil {
		return fmt.Errorf("generated subscription failed Mihomo validation and was not published: %w", err)
	}
	outputPath := "clash.yaml"
	if err := storage.AtomicWrite(outputPath, out, 0644); err != nil {
		return fmt.Errorf("failed to write subscription: %w", err)
	}
	regions := map[string]bool{}
	for _, node := range nodes {
		if node.Region != "" && node.Region != "OTHER" {
			regions[node.Region] = true
		}
	}
	message := "Subscription generated"
	if len(nodes) == 0 {
		message += " (no usable nodes; using DIRECT)"
	} else {
		message += fmt.Sprintf(" (%d nodes connected)", len(nodes))
	}
	io.WriteString(os.Stdout, fmt.Sprintf("Pangolin %s %s\nSources: %d (failed %d)\nCandidates: %d, published: %d, regions: %d\nOutput: %s\n", version.Version, message, len(sources), failedSources, candidateCount, publishedCount, len(regions), outputPath))
	if len(failureDetails) > 0 {
		io.WriteString(os.Stdout, "Source failure details: "+strings.Join(failureDetails, "; ")+"\n")
	}
	return nil
}
