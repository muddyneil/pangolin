package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/pangolin/pangolin/internal/app"
	"github.com/pangolin/pangolin/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "显示版本")
	configPath := flag.String("config", "", "配置文件路径")
	flag.Parse()
	if *showVersion {
		io.WriteString(os.Stdout, version.Version+"\n")
		return
	}
	if err := app.Run(*configPath); err != nil {
		io.WriteString(os.Stderr, "Pangolin 启动失败: "+err.Error()+"\n请修正配置后重新运行。\n")
		waitForConfirmation()
		os.Exit(1)
	}
}

func waitForConfirmation() {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return
	}
	io.WriteString(os.Stderr, "按 Enter 键退出。")
	_, _ = fmt.Fscanln(os.Stdin)
}
