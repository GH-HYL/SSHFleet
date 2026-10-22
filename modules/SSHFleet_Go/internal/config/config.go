// Package config 承载主干第 1 步：加载 ./config/SSHFleet.conf（TOML）。
//
// 三条硬规则：
//   - **所有字段必填**（字段要出现，值可留空），不写死任何默认值：缺字段与非法取值都在这层报错
//   - **未知字段零容忍**：解码后取未识别键，报错列出具体键名
//   - **配置里的路径一律本机绝对路径**，分隔符 `\` 与 `/` 通用、盘符统一大写，程序用时按 `/` 处理。
//     绝对路径的形态随平台变：盘符与 UNC 是 Windows 的形态，以 `/` 开头是 Unix 系的形态，两边不能互换
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

// Account 登录账号。
type Account struct {
	Port     int    `toml:"port"`
	User     string `toml:"user"`
	Password string `toml:"password"`
}

// Credential 凭据。password / key_password 的**值语义随 Encrypt 变**：
//   - Encrypt = false：值是密码 / 口令**本身**
//   - Encrypt = true：值是凭据文件的**绝对路径**
type Credential struct {
	Encrypt     bool   `toml:"encrypt"`
	Key         string `toml:"key"`
	KeyPassword string `toml:"key_password"`
	SecretDir   string `toml:"secret_dir"`
}

// Execution 执行身份与三个超时。
type Execution struct {
	Sudo            bool `toml:"sudo"`
	TimeoutConnect  int  `toml:"timeout_connect"`
	TimeoutExecute  int  `toml:"timeout_execute"`
	TimeoutTransfer int  `toml:"timeout_transfer"`
}

// Enable 功能开关。
type Enable struct {
	OutputToXlsx     bool `toml:"output_to_xlsx"`
	ResultsToXlsx    bool `toml:"results_to_xlsx"`
	ShowCategoryTips bool `toml:"show_category_tips"`
}

// Paths 产物的目录名与文件名。
//
// **已不开放配置**：字段定义原样留着，是为了将来能随时改回来（读取结构不动），
// 只在加载时拦一句"配置里不允许出现 [paths]"；恢复开放只需删掉 Load 里那道判断。
// 程序内部一律用 BuiltinPaths，不要去读用户的配置。
type Paths struct {
	ErrorKeywords     string `toml:"error_keywords"`
	DangerousKeywords string `toml:"dangerous_keywords"`
	PasswdKeywords    string `toml:"passwd_keywords"`
	Historys          string `toml:"history"`
	Tool              string `toml:"tool"`
	Exec              string `toml:"exec"`
	Asset             string `toml:"asset"`
	Output            string `toml:"output"`
	OutputXlsx        string `toml:"output_xlsx"`
	Report            string `toml:"report"`
	ResultsXlsx       string `toml:"results_xlsx"`
}

// Interactive 远端交互代填的匹配口径。两个开关同时管触发词与中止词。
type Interactive struct {
	Regex         bool `toml:"regex"`
	CaseSensitive bool `toml:"case_sensitive"`
}

// Upload 上传并发策略的三个阈值（单位字节）。
type Upload struct {
	SmallFile      int `toml:"small_file"`
	LargeFile      int `toml:"large_file"`
	MediumParallel int `toml:"medium_parallel"`
}

// Interpreter 解释器配置，只对命令模式（-c）与脚本模式（-s）生效。
//   - Command：命令模式使用的解释器；脚本模式后缀没配到时也回退到它
//   - Script： 脚本模式的「后缀 → 解释器」映射，键带点（如 ".sh"）
//
// 取值为一段命令名或路径（如 bash / python3 / /usr/bin/perl），原样交给目标机识别。
type Interpreter struct {
	Command string            `toml:"command"`
	Script  map[string]string `toml:"script"`
}

// Config 配置文件全集。
type Config struct {
	Account     Account     `toml:"account"`
	Credential  Credential  `toml:"credential"`
	Execution   Execution   `toml:"execution"`
	Enable      Enable      `toml:"enable"`
	Interactive Interactive `toml:"interactive"`
	// Env 是命令行下发前要在远端导入的环境变量（名字 → 值），空表 = 不导入。
	// 出厂配置给的是统一 locale / PATH / TERM 几条；拼装见 ssh.ExportPrefix。
	Env         map[string]string `toml:"env"`
	Interpreter Interpreter       `toml:"interpreter"`
	Upload      Upload            `toml:"upload"`
}

// BuiltinPaths 产物路径与文件名的内置取值（用户不可配）。
var BuiltinPaths = Paths{
	ErrorKeywords:     "./config/keywords_error.conf",
	DangerousKeywords: "./config/keywords_dangerous.conf",
	PasswdKeywords:    "./config/keywords_passwd.conf",
	Historys:          "history",
	Tool:              "SSHFleetTools.log",
	Exec:              "SSHFleetExec.log",
	Asset:             "assets",
	Output:            "terminal-output.txt",
	OutputXlsx:        "terminal-output.xlsx",
	Report:            "report.txt",
	ResultsXlsx:       "results.xlsx",
}

// Load 读取并校验配置文件，返回可直接使用的配置。
func Load(path string) (*Config, error) {
	if _, err := os.Stat(path); err != nil {
		wd, _ := os.Getwd()
		return nil, fmt.Errorf(
			"这个位置没有配置文件\n"+
				"      应该在这里：%s\n"+
				"      当前目录：%s\n"+
				"提示：请在本工具的解压目录下执行；配置文件必须放在该目录的 config 文件夹里", DisplayPath(path), wd)
	}

	var cfg Config
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, fmt.Errorf("解析 TOML 失败：%v\n提示：多数是引号或方括号没配对，照随包的配置文件核一遍", err)
	}

	// 已移除的段 / 键：给一句能照做的说明，不要混进"未识别字段"里
	if md.IsDefined("paths") {
		return nil, fmt.Errorf(
			"配置里不允许出现 [paths] 段\n" +
				"提示：把 [paths] 整段删掉即可，其余字段不用动")
	}
	// 环境变量段从 [execution.env] 提到了顶层，改用 [env]
	if md.IsDefined("execution", "env") {
		return nil, fmt.Errorf(
			"配置里不再有 [execution.env] 段\n" +
				"提示：把段名改成 [env] 即可——挪到顶层，里面一个个变量不用动")
	}

	// 未知字段零容忍：列出具体键名
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, k := range undecoded {
			keys = append(keys, k.String())
		}
		return nil, fmt.Errorf("配置包含未识别字段：%s\n提示：请使用随包提供的配置文件，不要增删任何字段", strings.Join(keys, ", "))
	}

	if err := validate(&cfg, md); err != nil {
		return nil, err
	}
	if err := resolveCredentialPaths(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DisplayPath 给用户看的路径形态：去掉开头的 "./"，它对用户没有意义，
// 留着会让人看不出这是相对当前目录的路径。
func DisplayPath(p string) string { return strings.TrimPrefix(p, "./") }

// validate 配置预检查：字段必须出现（值可留空） + 取值合规性。
// 预检查通过即代表配置自足，运行期一律取配置里的值。
func validate(cfg *Config, md toml.MetaData) error {
	required := []struct {
		key string
		val string
	}{
		{"account.port", definedStr(md, "account", "port")},
		{"account.user", definedStr(md, "account", "user")},
		{"account.password", definedStr(md, "account", "password")},
		{"credential.encrypt", definedStr(md, "credential", "encrypt")},
		{"credential.key", definedStr(md, "credential", "key")},
		{"credential.key_password", definedStr(md, "credential", "key_password")},
		{"credential.secret_dir", definedStr(md, "credential", "secret_dir")},
		{"execution.sudo", definedStr(md, "execution", "sudo")},
		{"execution.timeout_connect", definedStr(md, "execution", "timeout_connect")},
		{"execution.timeout_execute", definedStr(md, "execution", "timeout_execute")},
		{"execution.timeout_transfer", definedStr(md, "execution", "timeout_transfer")},
		{"enable.output_to_xlsx", definedStr(md, "enable", "output_to_xlsx")},
		{"enable.results_to_xlsx", definedStr(md, "enable", "results_to_xlsx")},
		{"enable.show_category_tips", definedStr(md, "enable", "show_category_tips")},
		{"interactive.regex", definedStr(md, "interactive", "regex")},
		{"interactive.case_sensitive", definedStr(md, "interactive", "case_sensitive")},
		{"env", definedStr(md, "env")},
		{"upload.small_file", definedStr(md, "upload", "small_file")},
		{"upload.large_file", definedStr(md, "upload", "large_file")},
		{"upload.medium_parallel", definedStr(md, "upload", "medium_parallel")},
		{"interpreter.command", definedStr(md, "interpreter", "command")},
	}
	var missing []string
	for _, r := range required {
		if r.val == "" {
			missing = append(missing, r.key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("配置缺少必填字段：%s\n提示：请使用随包提供的配置文件，不要删掉里面的任何一行", strings.Join(missing, ", "))
	}

	if cfg.Account.Port < 1 || cfg.Account.Port > 65535 {
		return fmt.Errorf("account.port 取值非法：%d（须为 1-65535）", cfg.Account.Port)
	}
	for name, v := range map[string]int{
		"execution.timeout_connect":  cfg.Execution.TimeoutConnect,
		"execution.timeout_execute":  cfg.Execution.TimeoutExecute,
		"execution.timeout_transfer": cfg.Execution.TimeoutTransfer,
	} {
		if v < 1 {
			return fmt.Errorf("%s 取值非法：%d（须为正整数，单位秒）", name, v)
		}
	}
	if cfg.Upload.SmallFile < 1 {
		return fmt.Errorf("upload.small_file 取值非法：%d（须为正整数，单位字节）", cfg.Upload.SmallFile)
	}
	if cfg.Upload.LargeFile < 1 {
		return fmt.Errorf("upload.large_file 取值非法：%d（须为正整数，单位字节）", cfg.Upload.LargeFile)
	}
	if cfg.Upload.LargeFile < cfg.Upload.SmallFile {
		return fmt.Errorf("上传阈值取值非法：large_file(%d) 不能小于 small_file(%d)", cfg.Upload.LargeFile, cfg.Upload.SmallFile)
	}
	if cfg.Upload.MediumParallel < 1 {
		return fmt.Errorf("upload.medium_parallel 取值非法：%d（须为正整数）", cfg.Upload.MediumParallel)
	}
	if err := validateEnv(cfg.Env); err != nil {
		return err
	}
	if err := validateInterpreter(&cfg.Interpreter); err != nil {
		return err
	}
	return nil
}

// envNameRe 合法 shell 变量名：字母或下划线开头，后接字母 / 数字 / 下划线。
var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateEnv 校验 [env] 并就地归一：
//   - 键名必须是合法 shell 变量名——`export` 只认这种名字，写别的（123、A-B）到远端会报
//     not a valid identifier、变量静默不生效，所以在启动阶段就拦下、点名；
//   - 空键或空值一律丢弃：空值 = 把这条临时关掉，按「没有这一条键值对」处理；
//   - 值不能以奇数个反斜杠结尾——值会被包进双引号下发，那个反斜杠会吃掉收尾的引号，
//     远端整条命令变成语法错（`unexpected EOF`），本地拦下比远端排查省事。
func validateEnv(env map[string]string) error {
	for name, value := range env {
		if name == "" || value == "" {
			delete(env, name)
			continue
		}
		if !envNameRe.MatchString(name) {
			return fmt.Errorf("env 里的 %q 不是合法的环境变量名\n"+
				"原因：shell 的 export 只认「字母或下划线开头，后接字母、数字、下划线」的名字，写别的到远端会报 not a valid identifier、变量不生效\n"+
				"提示：改成一个合法名字，例如 MY_VAR", name)
		}
		if trailingBackslashes(value)%2 == 1 {
			return fmt.Errorf("env 里的 %q 取值不能以反斜杠结尾，当前值：%q\n"+
				"原因：值会被包进双引号下发，结尾的反斜杠会吃掉那个收尾引号，远端整条命令变成语法错\n"+
				"提示：要一个反斜杠就写两个", name, value)
		}
	}
	return nil
}

// trailingBackslashes 值末尾连续反斜杠的个数（偶数个是安全的：两个凑成一个字面反斜杠）。
func trailingBackslashes(s string) int {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n
}

// validateInterpreter 校验解释器配置，并把脚本后缀键归一为小写（匹配时忽略大小写）。
//
// 取值只要求"配了就有值"：命令模式项不能空，脚本映射每条的后缀与解释器都不能空；
// 后缀须带点、不能重复（忽略大小写）。解释器本身允许写名字或路径，原样交给目标机，
// 不做可用性判断——能不能跑是目标机的事。
func validateInterpreter(it *Interpreter) error {
	it.Command = strings.TrimSpace(it.Command)
	if it.Command == "" {
		return fmt.Errorf("interpreter.command 不能为空\n提示：写命令模式（-c）要用的解释器，例如 bash")
	}
	if strings.ContainsAny(it.Command, "\r\n") {
		return fmt.Errorf("interpreter.command 不能含换行，当前值：%q", it.Command)
	}
	if len(it.Script) == 0 {
		return fmt.Errorf("interpreter.script 至少要配一条\n" +
			"提示：写成 \".sh\" = \"bash\" 这样，后缀带点、等号两边都要有值")
	}
	norm := make(map[string]string, len(it.Script))
	seen := make(map[string]string, len(it.Script))
	for rawKey, rawVal := range it.Script {
		key := strings.TrimSpace(rawKey)
		val := strings.TrimSpace(rawVal)
		if key == "" {
			return fmt.Errorf("interpreter.script 里有一条后缀是空的\n" +
				"提示：每条都要写成 \".sh\" = \"bash\"，后缀带点、等号两边都要有值")
		}
		if !strings.HasPrefix(key, ".") {
			return fmt.Errorf("interpreter.script 的后缀要以点开头，当前值：%q\n提示：写成 \".sh\"，不要写成 \"sh\"", key)
		}
		if val == "" {
			return fmt.Errorf("interpreter.script 里 %q 没有配解释器\n提示：等号右边要写解释器，例如 \".sh\" = \"bash\"", key)
		}
		if strings.ContainsAny(val, "\r\n") {
			return fmt.Errorf("interpreter.script 里 %q 的解释器不能含换行，当前值：%q", key, val)
		}
		lower := strings.ToLower(key)
		if prev, dup := seen[lower]; dup {
			return fmt.Errorf("interpreter.script 里后缀重复：%q 与 %q 是同一个后缀（大小写不区分）\n"+
				"提示：一个后缀只能配一个解释器", prev, key)
		}
		seen[lower] = key
		norm[lower] = val
	}
	it.Script = norm
	return nil
}

func definedStr(md toml.MetaData, path ...string) string {
	if md.IsDefined(path...) {
		return "1"
	}
	return ""
}

// resolveCredentialPaths 归一化配置里的路径字段。
//
//	secret_dir / key 恒为路径（取值不随加密开关变）
//	password / key_password 只在打开加密时是路径；不加密时是密码 / 口令本身，原样保留
func resolveCredentialPaths(cfg *Config) error {
	var err error
	if cfg.Credential.SecretDir, err = normalizeConfigPath("credential.secret_dir", cfg.Credential.SecretDir); err != nil {
		return err
	}
	if cfg.Credential.Key, err = normalizeConfigPath("credential.key", cfg.Credential.Key); err != nil {
		return err
	}
	if cfg.Credential.Encrypt {
		if cfg.Account.Password, err = normalizeConfigPath("account.password", cfg.Account.Password); err != nil {
			return err
		}
		if cfg.Credential.KeyPassword, err = normalizeConfigPath("credential.key_password", cfg.Credential.KeyPassword); err != nil {
			return err
		}
		return nil
	}
	cfg.Account.Password = strings.TrimSpace(cfg.Account.Password)
	cfg.Credential.KeyPassword = strings.TrimSpace(cfg.Credential.KeyPassword)
	return nil
}

// normalizeConfigPath 归一化一个配置路径：分隔符统一为 /、盘符转大写，且必须是**本机**绝对路径。
// 空值表示未配置，放行。不解析 ~：配置是长期备着的东西，写全路径。
//
// 报错按值的形态分四种，不压成一句：写 C:/Keys 的人写的是绝对路径，只是不是本机的——
// 「必须是绝对路径」只留给真的写了相对路径的那种。为什么这么分见下方分支。
func normalizeConfigPath(name, raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", nil
	}
	p = strings.ReplaceAll(p, `\`, "/")
	hasDrive := len(p) >= 2 && p[1] == ':' && isDriveLetter(p[0])
	if hasDrive {
		p = strings.ToUpper(p[:1]) + p[1:]
	}

	// 提示里只给本机形态的示例：同时摆两种，等于教用户写一个马上会被否掉的写法
	example, absHint := "/home/ops/.keys", "写成以 / 开头的本机路径"
	if isWindows() {
		example, absHint = "D:/Keys", "写成带盘符的本机路径"
	}
	pathErr := func(head, reason string) error {
		return fmt.Errorf("%s %s，当前值：%s\n原因：%s\n提示：%s，例如 %s",
			name, head, raw, reason, absHint, example)
	}

	// ~：与相对路径无关的一类（它既不是相对也不是绝对），单独成句
	if strings.HasPrefix(p, "~") {
		return "", fmt.Errorf(
			"%s 不支持 ~，当前值：%s\n"+
				"原因：~ 只在清单里能用，配置里要写全路径\n"+
				"提示：%s，例如 %s", name, raw, absHint, example)
	}

	if isWindows() {
		if filepath.IsAbs(p) {
			return p, nil
		}
		if strings.HasPrefix(p, "/") {
			return "", pathErr("不是本机的绝对路径", "本机是 Windows，这个写法没有盘符")
		}
		return "", pathErr("必须是绝对路径", "配置里的路径一律写全路径；相对路径只在清单里能用")
	}

	// 非 Windows：盘符与 UNC 都是 Windows 形态，本机用不了。UNC 归一化后以 // 开头，
	// 而 filepath.IsAbs 在 Unix 系只看开头是不是 /——不单独拦下，它会被当成 /服务器/共享 的本地路径，
	// 最后只报「找不到凭据文件」，看不出真因（\\?\ 设备路径同理落这一支）。
	if hasDrive {
		return "", pathErr("不是本机的绝对路径", "本机是 Unix 系系统，这是 Windows 盘符写法")
	}
	if strings.HasPrefix(p, "//") {
		return "", pathErr("不是本机的绝对路径", "本机是 Unix 系系统，这是 Windows 网络路径（UNC）写法")
	}
	if filepath.IsAbs(p) {
		return p, nil
	}
	return "", pathErr("必须是绝对路径", "配置里的路径一律写全路径；相对路径只在清单里能用")
}

// isWindows 决定路径形态的判定口径与报错措辞：绝对路径的形态随平台变。
func isWindows() bool { return runtime.GOOS == "windows" }

func isDriveLetter(b byte) bool { return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') }
