package parse

import (
	"fmt"
	"log"
	"os"

	"fixture/internal/gate"
)

// Run 一次犯规四处：写终端两句、自己退出、拿 log.Fatal 收口、造了别包的拒绝。
func Run(fail bool) {
	fmt.Println("类一")
	_ = os.Stderr
	os.Exit(1)
	if fail {
		log.Fatal("类三的第二种形状：打日志再退")
	}
	_ = gate.Refusal{}
}
