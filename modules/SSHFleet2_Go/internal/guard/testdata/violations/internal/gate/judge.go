package gate

import "fixture/internal/contract"

// Judge 在闸门里造了「起不来」——类五的坏样本。
func Judge() {
	_ = Refusal{}
	_ = contract.CannotStart{}
	// 换个写法造同一个类型：`new` 算构造点（ADR-0048）。
	_ = new(contract.CannotStart)
}
