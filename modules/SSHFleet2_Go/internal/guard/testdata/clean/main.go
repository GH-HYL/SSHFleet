package main

import (
	"os"

	"fixture/internal/console"
	"fixture/internal/parse"
)

func main() {
	_ = console.Line
	_ = parse.Run
	_ = os.Args // 2026-11-01 补：塔尖读 argv 已进白名单（ADR-0032 更新记录）
	os.Exit(0)
}
