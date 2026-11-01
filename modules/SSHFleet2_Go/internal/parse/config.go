package parse

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"sshfleet2/internal/message"
)

// 读配置：类型化、必填、未知字段、取值、路径归一。
//
// 三条硬规则照老版：所有字段必填（字段要出现，值可留空）；未知字段零容忍；配置里的路径
// 一律本机绝对路径，分隔符 \ 与 / 通用、盘符大写。
//
// 报错文案逐字照老版 config.go——它是使用者看得见的面，09 的逐字比对要过。

const (
	configPath      = "./config/SSHFleet.conf"
	shownConfigPath = "config/SSHFleet.conf"
)

// account 登录账号。
type account struct {
	Port     int
	User     string
	Password string
}

// credentialCfg 凭据。password／key_password 的值语义随 Encrypt 变：
// 关着加密时是密码本身，开着时是凭据文件的绝对路径。
type credentialCfg struct {
	Encrypt     bool
	Key         string
	KeyPassword string
	SecretDir   string
}

// execution 执行身份与三个超时。帮助那一屏也从这里取四格。
type execution struct {
	Sudo            bool
	TimeoutConnect  int
	TimeoutExecute  int
	TimeoutTransfer int
}

// enable 功能开关。
type enable struct {
	OutputToXlsx     bool
	ResultsToXlsx    bool
	ShowCategoryTips bool
}

// interactive 远端交互代填的匹配口径。
type interactive struct {
	Regex         bool
	CaseSensitive bool
}

// locale 提示语语言开关（编码不在这里，工具按目标机自行处理）。
type locale struct {
	English bool
}

// interpreter 解释器配置：命令模式用哪一个、脚本后缀映射到哪一个。
type interpreter struct {
	Command string
	Script  map[string]string
}

// upload 上传并发策略的三个阈值（单位字节）。
type upload struct {
	SmallFile      int
	LargeFile      int
	MediumParallel int
}

// settings 配置全集。
type settings struct {
	account       account
	credentialCfg credentialCfg
	Execution     execution
	enable        enable
	interactive   interactive
	Env           map[string]string
	locale        locale
	interpreter   interpreter
	upload        upload
}

// knownKeys 是认识的节与键：多一个就报「未识别字段」。
var knownKeys = map[string]map[string]bool{
	"account":     {"port": true, "user": true, "password": true},
	"credential":  {"encrypt": true, "key": true, "key_password": true, "secret_dir": true},
	"execution":   {"sudo": true, "timeout_connect": true, "timeout_execute": true, "timeout_transfer": true},
	"enable":      {"output_to_xlsx": true, "results_to_xlsx": true, "show_category_tips": true},
	"interactive": {"regex": true, "case_sensitive": true},
	"locale":      {"english": true},
	"interpreter": {"command": true},
	"upload":      {"small_file": true, "large_file": true, "medium_parallel": true},
}

// openSections 里的键名不校验（env 是任意变量名，interpreter.script 是任意后缀）。
var openSections = map[string]bool{"env": true, "interpreter.script": true}

// required 是必填清单：字段要出现，值可留空。照老版 22 条；env 那一项看的是「有没有 [env] 段」。
var required = [][2]string{
	{"account", "port"},
	{"account", "user"},
	{"account", "password"},
	{"credential", "encrypt"},
	{"credential", "key"},
	{"credential", "key_password"},
	{"credential", "secret_dir"},
	{"execution", "sudo"},
	{"execution", "timeout_connect"},
	{"execution", "timeout_execute"},
	{"execution", "timeout_transfer"},
	{"enable", "output_to_xlsx"},
	{"enable", "results_to_xlsx"},
	{"enable", "show_category_tips"},
	{"interactive", "regex"},
	{"interactive", "case_sensitive"},
	{"env", ""},
	{"locale", "english"},
	{"upload", "small_file"},
	{"upload", "large_file"},
	{"upload", "medium_parallel"},
	{"interpreter", "command"},
}

// Load 读配置并校验。返回的错误已经是给使用者看的那一段话（含「原因：」「提示：」）。
func load() (*settings, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, missingConfigError()
	}
	lines, err := tokenize(data)
	if err != nil {
		return nil, fmt.Errorf("解析 TOML 失败：%v\n提示：多数是引号或方括号没配对，照随包的配置文件核一遍", err)
	}
	raw := rawView(lines)

	// 已移除的段：给一句能照做的说明，不混进「未识别字段」。
	if _, ok := raw["paths"]; ok {
		return nil, fmt.Errorf("配置里不允许出现 [paths] 段\n提示：把 [paths] 整段删掉即可，其余字段不用动")
	}
	if _, ok := raw["execution.env"]; ok {
		return nil, fmt.Errorf("配置里不再有 [execution.env] 段\n提示：把段名改成 [env] 即可——挪到顶层，里面一个个变量不用动")
	}

	if unknown := unknownKeys(raw); len(unknown) > 0 {
		return nil, fmt.Errorf("配置包含未识别字段：%s\n提示：请使用随包提供的配置文件，不要增删任何字段", strings.Join(unknown, ", "))
	}
	if missing := missingKeys(raw); len(missing) > 0 {
		return nil, fmt.Errorf("配置缺少必填字段：%s\n提示：请使用随包提供的配置文件，不要删掉里面的任何一行", strings.Join(missing, ", "))
	}

	cfg, err := decode(raw)
	if err != nil {
		return nil, err
	}
	if err := validateValues(cfg); err != nil {
		return nil, err
	}
	if err := resolveCredentialPaths(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// missingConfigError 对位老实现 config.Load 的缺失文案（系统原话那句不翻新）。
func missingConfigError() error {
	wd, _ := os.Getwd()
	return fmt.Errorf(
		"这个位置没有配置文件\n"+
			"      应该在这里：%s\n"+
			"      当前目录：%s\n"+
			"提示：请在本工具的解压目录下执行；配置文件必须放在该目录的 config 文件夹里",
		shownConfigPath, wd)
}

// unknownKeys 列出不认识的键；开放表里的键名不校验。
func unknownKeys(raw rawConfig) []string {
	var out []string
	for section, keys := range raw {
		if openSections[section] {
			continue
		}
		known, ok := knownKeys[section]
		if !ok {
			for k := range keys {
				out = append(out, section+"."+k)
			}
			continue
		}
		for k := range keys {
			if !known[k] {
				out = append(out, section+"."+k)
			}
		}
	}
	sortStrings(out)
	return out
}

// missingKeys 列出没出现的必填项。env 那一项看的是「有没有 [env] 段」。
func missingKeys(raw rawConfig) []string {
	var out []string
	for _, r := range required {
		section, key := r[0], r[1]
		if key == "" {
			if _, ok := raw[section]; !ok {
				out = append(out, section)
			}
			continue
		}
		if _, ok := raw[section][key]; !ok {
			out = append(out, section+"."+key)
		}
	}
	return out
}

// decode 把原文表转成有类型的配置。取值合法性在下一步查。
func decode(raw rawConfig) (*settings, error) {
	cfg := &settings{}
	var err error

	if cfg.account.Port, err = intOf(raw, "account", "port"); err != nil {
		return nil, err
	}
	cfg.account.User = raw["account"]["user"]
	cfg.account.Password = raw["account"]["password"]

	if cfg.credentialCfg.Encrypt, err = boolOf(raw, "credential", "encrypt"); err != nil {
		return nil, err
	}
	cfg.credentialCfg.Key = raw["credential"]["key"]
	cfg.credentialCfg.KeyPassword = raw["credential"]["key_password"]
	cfg.credentialCfg.SecretDir = raw["credential"]["secret_dir"]

	if cfg.Execution.Sudo, err = boolOf(raw, "execution", "sudo"); err != nil {
		return nil, err
	}
	if cfg.Execution.TimeoutConnect, err = intOf(raw, "execution", "timeout_connect"); err != nil {
		return nil, err
	}
	if cfg.Execution.TimeoutExecute, err = intOf(raw, "execution", "timeout_execute"); err != nil {
		return nil, err
	}
	if cfg.Execution.TimeoutTransfer, err = intOf(raw, "execution", "timeout_transfer"); err != nil {
		return nil, err
	}

	if cfg.enable.OutputToXlsx, err = boolOf(raw, "enable", "output_to_xlsx"); err != nil {
		return nil, err
	}
	if cfg.enable.ResultsToXlsx, err = boolOf(raw, "enable", "results_to_xlsx"); err != nil {
		return nil, err
	}
	if cfg.enable.ShowCategoryTips, err = boolOf(raw, "enable", "show_category_tips"); err != nil {
		return nil, err
	}

	if cfg.interactive.Regex, err = boolOf(raw, "interactive", "regex"); err != nil {
		return nil, err
	}
	if cfg.interactive.CaseSensitive, err = boolOf(raw, "interactive", "case_sensitive"); err != nil {
		return nil, err
	}

	if cfg.locale.English, err = boolOf(raw, "locale", "english"); err != nil {
		return nil, err
	}

	if cfg.upload.SmallFile, err = intOf(raw, "upload", "small_file"); err != nil {
		return nil, err
	}
	if cfg.upload.LargeFile, err = intOf(raw, "upload", "large_file"); err != nil {
		return nil, err
	}
	if cfg.upload.MediumParallel, err = intOf(raw, "upload", "medium_parallel"); err != nil {
		return nil, err
	}

	cfg.Env = map[string]string{}
	for k, v := range raw["env"] {
		cfg.Env[k] = v
	}

	cfg.interpreter.Command = raw["interpreter"]["command"]
	cfg.interpreter.Script = map[string]string{}
	for k, v := range raw["interpreter.script"] {
		cfg.interpreter.Script[k] = v
	}
	return cfg, nil
}

// intOf 取一个整数键；认不出就报「取值非法」。
func intOf(raw rawConfig, section, key string) (int, error) {
	v := raw[section][key]
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("%s.%s 取值非法：%s（须为整数）", section, key, v)
	}
	return n, nil
}

// boolOf 取一个布尔键；只认 true／false。
func boolOf(raw rawConfig, section, key string) (bool, error) {
	v := strings.TrimSpace(raw[section][key])
	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("%s.%s 取值非法：%s（只认 true / false）", section, key, v)
}

// validateValues 取值合规性：端口、三个超时、上传阈值、[env]、[interpreter]。
func validateValues(cfg *settings) error {
	if cfg.account.Port < 1 || cfg.account.Port > 65535 {
		return fmt.Errorf("account.port 取值非法：%d（须为 1-65535）", cfg.account.Port)
	}
	for _, x := range []struct {
		name string
		val  int
	}{
		{"execution.timeout_connect", cfg.Execution.TimeoutConnect},
		{"execution.timeout_execute", cfg.Execution.TimeoutExecute},
		{"execution.timeout_transfer", cfg.Execution.TimeoutTransfer},
	} {
		if x.val < 1 {
			return fmt.Errorf("%s 取值非法：%d（须为正整数，单位秒）", x.name, x.val)
		}
	}
	if cfg.upload.SmallFile < 1 {
		return fmt.Errorf("upload.small_file 取值非法：%d（须为正整数，单位字节）", cfg.upload.SmallFile)
	}
	if cfg.upload.LargeFile < 1 {
		return fmt.Errorf("upload.large_file 取值非法：%d（须为正整数，单位字节）", cfg.upload.LargeFile)
	}
	if cfg.upload.LargeFile < cfg.upload.SmallFile {
		return fmt.Errorf("上传阈值取值非法：large_file(%d) 不能小于 small_file(%d)", cfg.upload.LargeFile, cfg.upload.SmallFile)
	}
	if cfg.upload.MediumParallel < 1 {
		return fmt.Errorf("upload.medium_parallel 取值非法：%d（须为正整数）", cfg.upload.MediumParallel)
	}
	if err := validateEnv(cfg.Env); err != nil {
		return err
	}
	return validateInterpreter(&cfg.interpreter)
}

// envNameRe 合法 shell 变量名：字母或下划线开头，后接字母／数字／下划线。
var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateEnv 校验 [env]：键名合法、空键空值丢弃、值不能以奇数个反斜杠结尾。
func validateEnv(env map[string]string) error {
	for name, value := range env {
		if name == "" || value == "" {
			delete(env, name)
			continue
		}
		if !envNameRe.MatchString(name) {
			return fmt.Errorf("%s", message.Problem(
				fmt.Sprintf("env 里的 %q 不是合法的环境变量名", name),
				"shell 的 export 只认「字母或下划线开头，后接字母、数字、下划线」的名字，写别的到远端会报 not a valid identifier、变量不生效",
				"改成一个合法名字，例如 MY_VAR"))
		}
		if trailingBackslashes(value)%2 == 1 {
			return fmt.Errorf("%s", message.Problem(
				fmt.Sprintf("env 里的 %q 取值不能以反斜杠结尾，当前值：%q", name, value),
				"值会被包进双引号下发，结尾的反斜杠会吃掉那个收尾引号，远端整条命令变成语法错",
				"要一个反斜杠就写两个"))
		}
	}
	return nil
}

// trailingBackslashes 值末尾连续反斜杠的个数（偶数个安全）。
func trailingBackslashes(s string) int {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n
}

// validateInterpreter 校验解释器配置，并把脚本后缀键归一小写。
func validateInterpreter(it *interpreter) error {
	it.Command = strings.TrimSpace(it.Command)
	if it.Command == "" {
		return fmt.Errorf("interpreter.command 不能为空\n提示：写命令模式（-c）要用的解释器，例如 bash")
	}
	if strings.ContainsAny(it.Command, "\r\n") {
		return fmt.Errorf("interpreter.command 不能含换行，当前值：%q", it.Command)
	}
	if len(it.Script) == 0 {
		return fmt.Errorf("interpreter.script 至少要配一条\n提示：写成 \".sh\" = \"bash\" 这样，后缀带点、等号两边都要有值")
	}
	norm := make(map[string]string, len(it.Script))
	seen := make(map[string]string, len(it.Script))
	for rawKey, rawVal := range it.Script {
		key := strings.TrimSpace(rawKey)
		val := strings.TrimSpace(rawVal)
		if key == "" {
			return fmt.Errorf("interpreter.script 里有一条后缀是空的\n提示：每条都要写成 \".sh\" = \"bash\"，后缀带点、等号两边都要有值")
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
			return fmt.Errorf("interpreter.script 里后缀重复：%q 与 %q 是同一个后缀（大小写不区分）\n提示：一个后缀只能配一个解释器", prev, key)
		}
		seen[lower] = key
		norm[lower] = val
	}
	it.Script = norm
	return nil
}

// resolveCredentialPaths 归一化配置里的路径字段。
func resolveCredentialPaths(cfg *settings) error {
	var err error
	if cfg.credentialCfg.Encrypt {
		if cfg.credentialCfg.SecretDir, err = normalizeConfigPath("credential.secret_dir", cfg.credentialCfg.SecretDir); err != nil {
			return err
		}
	}
	if cfg.credentialCfg.Key, err = normalizeConfigPath("credential.key", cfg.credentialCfg.Key); err != nil {
		return err
	}
	if cfg.credentialCfg.Encrypt {
		if cfg.account.Password, err = normalizeConfigPath("account.password", cfg.account.Password); err != nil {
			return err
		}
		if cfg.credentialCfg.KeyPassword, err = normalizeConfigPath("credential.key_password", cfg.credentialCfg.KeyPassword); err != nil {
			return err
		}
		return nil
	}
	cfg.account.Password = strings.TrimSpace(cfg.account.Password)
	cfg.credentialCfg.KeyPassword = strings.TrimSpace(cfg.credentialCfg.KeyPassword)
	return nil
}

// normalizeConfigPath 归一化一个配置路径：分隔符统一 /、盘符大写，且必须是本机绝对路径。
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

	example, absHint := "/home/ops/.keys", "写成以 / 开头的本机路径"
	if isWindows() {
		example, absHint = "D:/Keys", "写成带盘符的本机路径"
	}
	pathErr := func(head, reason string) error {
		return fmt.Errorf("%s", message.Problem(
			fmt.Sprintf("%s %s，当前值：%s", name, head, raw),
			reason,
			fmt.Sprintf("%s，例如 %s", absHint, example)))
	}

	if strings.HasPrefix(p, "~") {
		return "", pathErr("不支持 ~", "~ 不会被展开，写什么就按什么找")
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

// isWindows 决定路径形态的判定口径与报错措辞。
func isWindows() bool { return runtime.GOOS == "windows" }

func isDriveLetter(b byte) bool { return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') }

// sortStrings 小排序：未识别字段的报错要稳定，免得同一份配置两次报出不同顺序。
func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}
