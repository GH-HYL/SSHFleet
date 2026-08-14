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
	Output          string  `json:"output"`    // base64 编码
	ConnectCostTime float64 `json:"connect_cost_time"`
	ExecCostTime    float64 `json:"exec_cost_time"`
	Error           *string `json:"error"` // 成功时 null
}

// DoneResponse SSE 完成标记
// 只报节点总数，节点成功/失败由调用方根据 result 流自行统计（Go 端不统计、不处理）
type DoneResponse struct {
	Type  string `json:"type"`
	Total int    `json:"total"`
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
	Output          string  `json:"output"`    // base64 编码
	ConnectCostTime float64 `json:"connect_cost_time"`
	ExecCostTime    float64 `json:"exec_cost_time"`
	Error           *string `json:"error"`
	TotalBytes      int64   `json:"total_bytes"` // 实际成功上传的字节数
	TotalFiles      int     `json:"total_files"`
	SuccessFiles    int     `json:"success_files"`
	FailedFiles     int     `json:"failed_files"`
}

// DownloadResult 下载结果（单个节点）
// 字段与 UploadResult 保持一致（JSON 结构完全相同，调用方按同一结构解析），
// 但语义为下载：TotalBytes 是实际下载字节数，SuccessFiles/FailedFiles 是下载成功/失败文件数。
type DownloadResult struct {
	Type            string  `json:"type"`
	Seq             int     `json:"seq"`
	IP              string  `json:"ip"`
	Port            int     `json:"port"`
	User            string  `json:"user"`
	ConnectSuccess  bool    `json:"connect_success"`
	ExitCode        *int    `json:"exit_code"` // nil 表示未执行（连接失败）
	Output          string  `json:"output"`    // base64 编码
	ConnectCostTime float64 `json:"connect_cost_time"`
	ExecCostTime    float64 `json:"exec_cost_time"`
	Error           *string `json:"error"`
	TotalBytes      int64   `json:"total_bytes"` // 实际成功下载的字节数
	TotalFiles      int     `json:"total_files"`
	SuccessFiles    int     `json:"success_files"`
	FailedFiles     int     `json:"failed_files"`
}
