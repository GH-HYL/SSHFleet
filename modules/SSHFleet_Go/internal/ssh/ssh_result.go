package ssh

// ExecResult 表示单个节点的执行结果
type ExecResult struct {
	Type            string  `json:"type"`
	Seq             int     `json:"seq"`
	IP              string  `json:"ip"`
	Port            int     `json:"port"`
	User            string  `json:"user"`
	ConnectSuccess  bool    `json:"connect_success"`
	ExitCode        *int    `json:"exit_code"` // nil 表示未执行（连接失败）
	Output          string  `json:"output"` // base64 编码
	ConnectCostTime float64 `json:"connect_cost_time"`
	ExecCostTime    float64 `json:"exec_cost_time"`
	Error           *string `json:"error"` // 成功时 null
}

// DoneResponse SSE 完成标记
type DoneResponse struct {
	Type    string `json:"type"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
}

// UploadResult 上传结果（单个节点）
type UploadResult struct {
	Type            string  `json:"type"`
	Seq             int     `json:"seq"`
	IP              string  `json:"ip"`
	Port            int     `json:"port"`
	User            string  `json:"user"`
	ConnectSuccess  bool    `json:"connect_success"`
	ExitCode        *int    `json:"exit_code"` // nil 表示未执行（连接失败）
	Output          string  `json:"output"` // base64 编码
	ConnectCostTime float64 `json:"connect_cost_time"`
	ExecCostTime    float64 `json:"exec_cost_time"`
	Error           *string `json:"error"`
	TotalBytes      int64   `json:"total_bytes"`
	TotalFiles      int     `json:"total_files"`
	SuccessFiles    int     `json:"success_files"`
	FailedFiles     int     `json:"failed_files"`
}
