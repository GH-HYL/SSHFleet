package contract

// 运行上下文（run metadata）那一格的形状与封法常量。
//
// 它是「第二类格子」（ADR-0035 决定九）：不算数据格，因此不参与「后一张不许比前一张宽」
// 的计法。解析站把原值封成一段字节放进去，一路原样传，**只有报告站解开**。
//
// 为什么封：`report.txt` 第一段要的「执行命令 / 执行参数」必须是使用者敲的原值，重新打包
// 一遍就不叫原值了；可原值一路直传，中间站就成了「拿到了、不用、也不处理，原样转手」——
// 正是「最窄」要禁的样子。封成看不懂的字节，拦的是顺手，不是存心（ADR-0035 决定九）。
//
// 封法与密钥都不在这里：契约只放类型与常量，一个函数都没有。哪两处封／解见
// internal/parse/runmeta.go 与 internal/report/runmeta.go，密钥各写一次。
type RunMeta []byte

// RunMetaBody 是封进去之前的那份东西：报告站写 `report.txt` 第一段要用到的一切原值。
//
// 它是「解析站封进去的一份副本」，不是第二处算出来的值——全工程只有解析站写它一次。
// 报告站之外谁也不该读它，也读不到。
type RunMetaBody struct {
	// Argv 是使用者敲的整条命令行（含程序名）。`report.txt` 的【执行命令】那一行由它来，
	// 而且**印之前要遮**：内联清单把密码写在这条命令行里。
	Argv []string `json:"argv"`

	// Mode 是这次走哪一种模式，报告第一段「执行模式：」那一行用它。
	Mode Mode `json:"mode"`

	// 各模式的原值：命令原文、脚本路径、上传源、目标位置。空的那些就是这次没用到的。
	Command    string `json:"command,omitempty"`
	ScriptPath string `json:"script_path,omitempty"`
	Source     string `json:"source,omitempty"`
	Target     string `json:"target,omitempty"`

	// Answers 是 `-a` 的原值（内联那一条，或代填文件的路径）。
	Answers string `json:"answers,omitempty"`

	// 执行参数：并发与三个超时。报告站印的是这次实际生效的值，不是命令行上写了什么。
	Concurrency     int `json:"concurrency,omitempty"`
	TimeoutConnect  int `json:"timeout_connect,omitempty"`
	TimeoutExecute  int `json:"timeout_execute,omitempty"`
	TimeoutTransfer int `json:"timeout_transfer,omitempty"`
}

// 封法常量：两处（封与解）共用，改格式只改这里一处。
const (
	// RunMetaVersion 是那段字节的开头一版号；格式改了就加一，老的那一段自然解不开。
	RunMetaVersion byte = 1
	// RunMetaNonceSize 是 nonce 的字节数（GCM 标准长度）。
	RunMetaNonceSize = 12
	// RunMetaTagSize 是 GCM 认证标签的字节数。
	RunMetaTagSize = 16
)
