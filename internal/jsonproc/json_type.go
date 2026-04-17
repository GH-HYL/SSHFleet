package jsonproc

// RequestData 请求数据结构
type RequestData struct {
	CommandArgs   *CommandArgs        `json:"args"`           // 命令行参数
	Config        *Config             `json:"config"`         // 配置信息
	NodesInfo     []*NodeInfo         `json:"nodesinfo"`      // 节点信息列表
	MonitorInfo   string              `json:"monitor_info"`   // 监视器IP端口
	ExecLogDir    string              `json:"exec_log_dir"`   // 执行日志目录
	FinalCommand  string              `json:"final_command"`  // 拼接后的命令
	ErrorKeywords map[string][]string `json:"error_keywords"` // 错误关键词JSON字符串
}

// CommandArgs 命令行参数
type CommandArgs struct {
	Command        string `json:"c"`              // 要执行的命令
	Script         string `json:"s"`              // 脚本路径
	Upload         string `json:"u"`              // 上传目标文件路径
	Download       string `json:"d"`              // 下载目标目录
	Zip            bool   `json:"z"`              // 是否压缩
	File           string `json:"f"`              // IP列表文件
	Path           string `json:"p"`              // 远端目标路径
	Mode           string `json:"m"`              // 执行模式
	TimeoutExecute int    `json:"t"`              // 执行超时时间
	TimeoutConnect int    `json:"T"`              // 连接超时时间
	Number         int    `json:"n"`              // 并发数量
	Env            string `json:"e"`              // 环境变量
	PrivateKey     string `json:"k"`              // 私钥路径
	Remark         string `json:"r"`              // 备注
	Disinteractive bool   `json:"disinteractive"` // 是否非交互式
}

// NodeInfo 节点信息
type NodeInfo struct {
	IP       string `json:"ip"`       // 节点IP地址
	Port     int    `json:"port"`     // SSH端口
	User     string `json:"user"`     // 用户名
	Password string `json:"password"` // 密码
}

// Config 配置信息
type Config struct {
	Account   *Account   `json:"account"`   // 账户信息
	Execution *Execution `json:"execution"` // 执行配置
	Paths     *Paths     `json:"paths"`     // 路径配置
}

// Account 账户信息
type Account struct {
	Port     int    `json:"port"`     // SSH端口
	User     string `json:"user"`     // 用户名
	Password string `json:"password"` // 密码
}

// Execution 执行配置
type Execution struct {
	Mode            string `json:"mode"`             // 执行模式
	TimeoutExecute  int    `json:"timeout_execute"`  // 执行超时时间
	TimeoutTransfer int    `json:"timeout_transfer"` // 传输超时时间
	TimeoutConnect  int    `json:"timeout_connect"`  // 连接超时时间
	PrivateKey      string `json:"private_key"`      // 私钥路径
}

// Paths 路径配置
type Paths struct {
	Jsons *Jsons `json:"jsons"` // 错误分类JSON文件路径
	Exe   *Exe   `json:"exe"`   // 可执行文件路径
	Logs  *Logs  `json:"logs"`  // 日志路径
	Files *Files `json:"files"` // 文件路径
}

type Jsons struct {
	ErrorKeywords string `json:"error_keywords"`
}

// Exe 可执行文件路径
type Exe struct {
	BatchExe string `json:"batch_exe"` // 批处理执行文件路径
}

// Logs 日志路径
type Logs struct {
	Zip      string `json:"zip"`      // 压缩文件路径
	Historys string `json:"historys"` // 历史记录路径
	Tool     string `json:"tool"`     // 工具日志路径
	Exec     string `json:"exec"`     // 执行日志路径
}

// Files 文件路径
type Files struct {
	Asset      string `json:"asset"`       // 资产文件路径
	Output     string `json:"output"`      // 输出文件路径
	OutputXlsx string `json:"output_xlsx"` // Excel输出文件路径
	Report     string `json:"report"`      // 报告文件路径
	Results    string `json:"results"`     // 结果文件路径
}
