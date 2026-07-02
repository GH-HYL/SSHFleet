package ssh

// ExecResult 表示单个节点的执行结果
type ExecResult struct {
	Seq             int     `json:"seq"`
	IP              string  `json:"ip"`
	Port            int     `json:"port"`
	User            string  `json:"user"`
	ConnectSuccess  bool    `json:"connect_success"`
	ExitCode        int     `json:"exit_code"`
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
	Seq             int              `json:"seq"`
	IP              string           `json:"ip"`
	Port            int              `json:"port"`
	User            string           `json:"user"`
	ConnectSuccess  bool             `json:"connect_success"`
	ConnectCostTime float64          `json:"connect_cost_time"`
	Files           []FileUploadItem `json:"files"`
	TotalFiles      int              `json:"total_files"`
	SuccessFiles    int              `json:"success_files"`
	FailedFiles     int              `json:"failed_files"`
	Error           *string          `json:"error"`
}

// FileUploadItem 单个文件上传结果
type FileUploadItem struct {
	FileName   string  `json:"file_name"`
	FilePath   string  `json:"file_path"`
	RemotePath string  `json:"remote_path"`
	FileSize   int64   `json:"file_size"`
	Success    bool    `json:"success"`
	CostTime   float64 `json:"cost_time"`
	Error      *string `json:"error"`
}
