package parse

import (
	"fmt"
	"os"

	"fixture/internal/gate"
)

// Run 一次犯规三处：写终端两句、自己退出、造了别包的拒绝。
func Run() {
	fmt.Println("类一")
	_ = os.Stderr
	os.Exit(1)
	_ = gate.Refusal{}
}
