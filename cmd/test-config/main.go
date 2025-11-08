package main

import (
	"cyberstrike-ai/internal/config"
	"fmt"
	"os"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		fmt.Printf("❌ 加载配置失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ 配置加载成功\n")
	fmt.Printf("   工具目录: %s\n", cfg.Security.ToolsDir)
	fmt.Printf("   工具数量: %d\n", len(cfg.Security.Tools))

	if len(cfg.Security.Tools) > 0 {
		fmt.Printf("\n   已加载的工具:\n")
		for _, tool := range cfg.Security.Tools {
			status := "❌ 禁用"
			if tool.Enabled {
				status = "✅ 启用"
			}
			shortDesc := tool.ShortDescription
			if shortDesc == "" {
				shortDesc = "(无简短描述，将自动提取)"
			}
			fmt.Printf("   %s %s\n", status, tool.Name)
			fmt.Printf("      简短描述: %s\n", shortDesc)
			if len(tool.Description) > 100 {
				fmt.Printf("      详细描述: %s...\n", tool.Description[:100])
			} else {
				fmt.Printf("      详细描述: %s\n", tool.Description)
			}
			fmt.Printf("      参数数量: %d\n", len(tool.Parameters))
			fmt.Println()
		}
	} else {
		fmt.Printf("   ⚠️  未加载任何工具\n")
	}
}

