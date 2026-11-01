package message

import (
	"fixture/internal/parse"
)

// Say 借了解析站——类二的坏样本（横切包反向依赖站）。
func Say() string {
	_ = parse.Run
	return ""
}
