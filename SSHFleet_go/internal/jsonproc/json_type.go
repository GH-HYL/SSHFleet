package jsonproc

// ExecuteRequest HTTP 请求体
type ExecuteRequest struct {
	Command string     `json:"command"`
	Options Options    `json:"options"`
	Nodes   []NodeInfo `json:"nodes"`
}

// Options 执行配置
type Options struct {
	Concurrency    int  `json:"concurrency"`
	ConnectTimeout int  `json:"connect_timeout"`
	ExecTimeout    int  `json:"exec_timeout"`
	Sudo           bool `json:"sudo"`
}

// NodeInfo 节点信息
type NodeInfo struct {
	Seq      int    `json:"seq"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
}

// UploadRequest 上传请求
type UploadRequest struct {
	FilePath   string     `json:"file_path"`
	RemotePath string     `json:"remote_path"`
	Options    Options    `json:"options"`
	Nodes      []NodeInfo `json:"nodes"`
}
