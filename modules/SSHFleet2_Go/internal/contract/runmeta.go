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

// RunMetaBody 是封进去之前的那份东西：报告站出那两样（屏幕统计块、`report.txt`）要用到的一切原值。
//
// 头一段是「使用者敲了什么」，后两样是「这次生效的呈现口径」——三类都只有解析站取得到，
// 而报告站只收得到第三张纸，所以一次收齐封进来（开发者 2026-11-02：「他要什么，解析站就给
// 什么，一个解析站直接全获取了，直接打包好加密」）。
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

	// Answers 是解析后的代填表（不是 `-a` 的原值：原值在 Argv 里）。
	//
	// 报告站印「代填 N：内容 → 触发词」那几行要的就是这张表，而 `-a` 的解析是解析站已经
	// 做过的活——让报告站回源头重读一遍等于把同一件事做两次。
	Answers []Answer `json:"answers,omitempty"`

	// NewPassword 是改密的新密码**明文**（加密开着时由解析站从密文文件解出来）。
	//
	// 装明文而不是装遮好的形态：`mask` 那条结构决定是「谁要谁现算，不产生字段、不往下传」，
	// 把已经遮好的值装进纸里等于凭空多一格。报告站印之前自己调 `mask` 算一次。
	NewPassword string `json:"new_password,omitempty"`

	// 执行参数：并发与三个超时。报告站印的是这次实际生效的值，不是命令行上写了什么。
	//
	// **并发数不在这里**：它会被闸门改，而这段密文在解析站返回前就封好了，改之前的值印出来
	// 就与老版对不上。它走第三张纸（`ResultPaper.Concurrency` → `Ledger.Concurrency`）。
	// 三个超时不会被改，仍留在这里。
	TimeoutConnect  int `json:"timeout_connect,omitempty"`
	TimeoutExecute  int `json:"timeout_execute,omitempty"`
	TimeoutTransfer int `json:"timeout_transfer,omitempty"`

	// ShowCategoryTips 是配置 `enable.show_category_tips` 的原值：屏幕上统计块末尾那段
	// 「提示：」与 `report.txt` 里那一段都由它决定出不出（关掉时屏幕退回「常见退出码」一行）。
	//
	// 报告站手上只有第三张纸与这一段密文，读不到配置——而两处产出都要它，所以随原值一起封进来
	// （开发者 2026-11-02：报告站要什么，解析站一次收齐封好）。
	ShowCategoryTips bool `json:"show_category_tips,omitempty"`

	// ErrorKeywordsPath 是判据文件的展示路径（`config/keywords_error.conf`）：屏幕上
	// 「未归类」那三行引导里有一句「可把它的特征词补进 〈这里〉」。
	//
	// 存的是**已经按展示形态算好**的值（老版 `config.DisplayPath` 去掉开头的 `./`），
	// 报告站只管印——它不该为了印一行路径去认路径怎么归一。
	ErrorKeywordsPath string `json:"error_keywords_path,omitempty"`

	// OutputToXlsx 与 ResultsToXlsx 是配置 `[enable]` 那两个开关的原值：报告站出两张
	// 表格时按它决定做不做。它们**不进纸一的数据格**——执行站不读它们，装了就是同一份
	// 原值两个来源；也不由塔尖单递（ADR-0035 决定八）。与上面那两格同族、同一条理由。
	OutputToXlsx  bool `json:"output_to_xlsx,omitempty"`
	ResultsToXlsx bool `json:"results_to_xlsx,omitempty"`

	// AnswerRaw 是 `-a` 的原值：一份代填文件的路径，或一条内联代填。
	//
	// 备份原料时报告站要它（代填文件的原件要进 `assets/`），而「是不是一个存在的普通文件」
	// 由报告站备份时现判——与清单那份同一处判据（空值、目录、找不到，一律跳过）。
	AnswerRaw string `json:"answer_raw,omitempty"`
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
