package main

import (
	"flag"
	"fmt"
	"os"
	"identity"
)

func main() {
	catalog := flag.String("catalog", "", "目录 JSON（必填）")
	referrals := flag.String("referrals", "", "邀请配置，可省略")
	out := flag.String("out", "", "产物目录（必填）")
	flag.Parse()
	if *catalog == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	if e := identity.Generate(*catalog, *referrals, *out); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	fmt.Println("静态导航已生成")
}
