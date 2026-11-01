// eawgen：把 x/text 的东亚宽（W/F）区间扫出来，生成新工程 message 包要的区间表。
// 只在 build/ 沙盒里跑一次，产物贴进 modules/SSHFleet2_Go/internal/message/width.go。
package main

import (
	"fmt"
	"golang.org/x/text/width"
)

func main() {
	inRun := false
	start := rune(0)
	for r := rune(0); r <= 0x10FFFF; r++ {
		k := width.LookupRune(r).Kind()
		wide := k == width.EastAsianWide || k == width.EastAsianFullwidth
		if wide && !inRun {
			start = r
			inRun = true
		} else if !wide && inRun {
			fmt.Printf("\t{0x%04X, 0x%04X},\n", start, r-1)
			inRun = false
		}
	}
	if inRun {
		fmt.Printf("\t{0x%04X, 0x%04X},\n", start, rune(0x10FFFF))
	}
}
