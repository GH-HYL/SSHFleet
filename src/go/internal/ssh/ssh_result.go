package ssh

import (
	"strings"
)

// ExecResult 命令执行结果
type ExecResult struct {
	IP   string `json:"ip"`   // 目标IP
	Port int    `json:"port"` // 端口
	User string `json:"user"` // 用户名

	ConnectSuccess bool   `json:"connect_success"` // 连接是否成功
	ConnectMsg     string `json:"connect_msg"`     // 连接信息

	//Command  string `json:"command"`   // 执行的命令
	Output   string `json:"output"`    // 输出内容
	ExitCode int    `json:"exit_code"` // 退出码
	ExecBool bool   `json:"exit_bool"` // 执行是否成功

	ConnCostTime float64 `json:"conn_cost_time"` // 连接耗时(秒)
	ExecCostTime float64 `json:"exec_cost_time"` // 执行耗时(秒)

	ResultCategory string `json:"result_category"` // 结果分类
}

var ErrorKeywords map[string][]string

// ClassifyError 错误分类
func ClassifyError(err error) string {
	if err == nil {
		return "错误信息为空"
	}
	// 小写处理
	errLow := strings.ToLower(err.Error())
	// 遍历错误map
	for category, keywords := range ErrorKeywords {
		// 遍历关键词
		for _, keyword := range keywords {
			if strings.Contains(errLow, keyword) {
				return category
			}
		}
	}
	return "错误未分类"
}
