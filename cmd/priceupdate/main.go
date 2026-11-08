package main

import (
	"context"
	"flag"
	"fmt"
	"identity"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"time"
)

func runWebCapture(catalog, configPath string) {
	cfg, e := identity.LoadPriceConfig(configPath)
	if e != nil || cfg.WebPrices == "" {
		return
	}
	root := filepath.Dir(filepath.Dir(catalog))
	script := filepath.Join(root, "tools", "web-prices.mjs")
	if _, e = os.Stat(script); e != nil {
		return
	}
	node, e := exec.LookPath("node")
	if e != nil {
		node = `D:\node.exe`
		if _, e = os.Stat(node); e != nil {
			fmt.Fprintln(os.Stderr, "网页价格抓取跳过：找不到 node")
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, script, "--auto", "--out", cfg.WebPrices, "--profile", filepath.Join(root, ".cache", "web-prices-profile"))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if e = cmd.Run(); e != nil {
		fmt.Fprintln(os.Stderr, "网页价格抓取未完成（不影响公开 API 采集）：", e)
	}
}

func main() {
	config := flag.String("config", "", "私有分析 API 配置（必填）")
	catalog := flag.String("catalog", "", "公开目录（必填）")
	referrals := flag.String("referrals", "", "邀请配置，可省略")
	out := flag.String("out", "", "导航输出目录（必填）")
	watch := flag.Bool("watch", false, "持续运行，按上次尝试时间每 24 小时更新")
	flag.Parse()
	for _, p := range []*string{config, catalog, out} {
		if *p == "" {
			flag.Usage()
			os.Exit(2)
		}
		abs, e := filepath.Abs(*p)
		if e != nil {
			fmt.Fprintln(os.Stderr, "路径无效")
			os.Exit(2)
		}
		*p = abs
	}
	if *referrals != "" {
		abs, e := filepath.Abs(*referrals)
		if e != nil {
			os.Exit(2)
		}
		*referrals = abs
	}
	if _, e := identity.LoadPriceConfig(*config); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	for {
		if *watch {
			c, e := identity.LoadCatalog(*catalog)
			if e != nil {
				fmt.Fprintln(os.Stderr, "目录读取失败")
				os.Exit(1)
			}
			if c.PriceUpdate != nil {
				next, e := time.Parse(time.RFC3339, c.PriceUpdate.NextUpdateAt)
				if e == nil && time.Until(next) > 0 {
					fmt.Println("下次价格更新：", next.Local().Format(time.RFC3339))
					timer := time.NewTimer(time.Until(next))
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
				}
			}
		}
		fmt.Println("开始拉取各渠道接码对象列表与公开价格；API key 不写入输出")
		runWebCapture(*catalog, *config)
		info, e := identity.UpdatePrices(ctx, *config, *catalog)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
		}
		if info.Saved {
			fmt.Printf("已拉取 %d 条渠道接码对象记录，核验 %d 条报价，%d 个来源失败，%d 个来源价格不完整\n", info.UpdatedServices, info.UpdatedOffers, len(info.Failures), len(info.PriceWarnings))
			if ge := identity.Generate(*catalog, *referrals, *out); ge != nil {
				fmt.Fprintln(os.Stderr, "价格目录已保存，导航生成未完成；请检查输出目录后重新生成导航")
				if !*watch {
					os.Exit(3) // Catalog committed; navigation publication failed.
				}
			} else {
				fmt.Println("用户导航已更新")
			}
		}
		if !*watch {
			if e != nil {
				if info.Saved {
					os.Exit(4) // Failure summary committed and navigation regenerated.
				}
				os.Exit(1)
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		if !info.Saved {
			fmt.Fprintln(os.Stderr, "未完成更新，24 小时后重试")
			timer := time.NewTimer(identity.PriceInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}
