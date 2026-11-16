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

// 配置路径这串在密钥包里也写了一份（它只读自己那一格加密开关，不掺和主流程）。
// 两边不许互相 import，所以认下这两份（开发者 2026-11-06 定）：**改这里要一起改那边**。
const (
	configPath      = "./config/SSHFleet.conf"
	shownConfigPath = message.ConfigShownPath
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

// Load 读配置并校验。返回的错误已经是给使用者看的那一段话（三行套由 message 拼好，这里只给内容）。
func load() (*settings, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, missingConfigError()
	}
	lines, err := tokenize(data)
	if err != nil {
		return nil, fmt.Errorf("%s", message.TOMLParseFailed(err.Error()))
	}
	raw := rawView(lines)

	// 已移除的段：给一句能照做的说明，不混进「未识别字段」。
	if _, ok := raw["paths"]; ok {
		return nil, fmt.Errorf("%s", message.ConfigPathsSectionForbidden())
	}
	if _, ok := raw["execution.env"]; ok {
		return nil, fmt.Errorf("%s", message.ConfigExecutionEnvSectionRenamed())
	}

	if unknown := unknownKeys(raw); len(unknown) > 0 {
		return nil, fmt.Errorf("%s", message.ConfigUnknownFields(strings.Join(unknown, ", ")))
	}
	if missing := missingKeys(raw); len(missing) > 0 {
		return nil, fmt.Errorf("%s", message.ConfigMissingFields(strings.Join(missing, ", ")))
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
	return fmt.Errorf("%s", message.ConfigNotInWorkDir(shownConfigPath, wd))
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
	if cfg.credentialCfg.Encrypt, err = boolOf(raw, "credential", "encrypt"); err != nil {
		return nil, err
	}
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

	// 字符串那一批：值必须写成带引号的字符串（见 stringOf）。
	for _, f := range []struct {
		section, key string
		dst          *string
	}{
		{"account", "user", &cfg.account.User},
		{"account", "password", &cfg.account.Password},
		{"credential", "key", &cfg.credentialCfg.Key},
		{"credential", "key_password", &cfg.credentialCfg.KeyPassword},
		{"credential", "secret_dir", &cfg.credentialCfg.SecretDir},
		{"interpreter", "command", &cfg.interpreter.Command},
	} {
		v, err := stringOf(raw, f.section, f.key)
		if err != nil {
			return nil, err
		}
		*f.dst = v
	}

	cfg.Env = map[string]string{}
	for k := range raw["env"] {
		s, err := stringOf(raw, "env", k)
		if err != nil {
			return nil, err
		}
		cfg.Env[k] = s
	}

	cfg.interpreter.Script = map[string]string{}
	for k := range raw["interpreter.script"] {
		s, err := stringOf(raw, "interpreter.script", k)
		if err != nil {
			return nil, err
		}
		cfg.interpreter.Script[k] = s
	}
	return cfg, nil
}

// intOf 取一个整数键；认不出就报「取值非法」。
func intOf(raw rawConfig, section, key string) (int, error) {
	v := raw[section][key]
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("%s", message.ValueInvalidBecause(section+"."+key, v, message.ReasonNotInt))
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
	return false, fmt.Errorf("%s", message.ValueInvalidBecause(section+"."+key, v, message.ReasonNotBool))
}

// stringOf 取一个字符串键：值必须写成带引号的字符串（'…' 或 "…"）。
//
// 「要不要引号」看值的类型——数字、布尔不带，字符串一律带。老版用 TOML 库，
// `password = /tmp/x` 这种裸值当场就是语法错；这里补上同一道判，口径不能松。
func stringOf(raw rawConfig, section, key string) (string, error) {
	v := strings.TrimSpace(raw[section][key])
	if isQuoted(v) {
		return unquote(v), nil
	}
	return "", fmt.Errorf("%s", message.ConfigStringValueUnquoted(section+"."+key, v))
}

// validateValues 取值合规性：端口、三个超时、上传阈值、[env]、[interpreter]。
func validateValues(cfg *settings) error {
	if cfg.account.Port < 1 || cfg.account.Port > 65535 {
		return fmt.Errorf("%s", message.ValueInvalidBecause("account.port", strconv.Itoa(cfg.account.Port), message.ReasonPort))
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
			return fmt.Errorf("%s", message.ValueInvalidBecause(x.name, strconv.Itoa(x.val), message.ReasonPositiveSeconds))
		}
	}
	if cfg.upload.SmallFile < 1 {
		return fmt.Errorf("%s", message.ValueInvalidBecause("upload.small_file", strconv.Itoa(cfg.upload.SmallFile), message.ReasonPositiveBytes))
	}
	if cfg.upload.LargeFile < 1 {
		return fmt.Errorf("%s", message.ValueInvalidBecause("upload.large_file", strconv.Itoa(cfg.upload.LargeFile), message.ReasonPositiveBytes))
	}
	if cfg.upload.LargeFile < cfg.upload.SmallFile {
		return fmt.Errorf("%s", message.ValueInvalidTight("上传阈值",
			message.UploadThresholdOrder(cfg.upload.LargeFile, cfg.upload.SmallFile)))
	}
	if cfg.upload.MediumParallel < 1 {
		return fmt.Errorf("%s", message.ValueInvalidBecause("upload.medium_parallel", strconv.Itoa(cfg.upload.MediumParallel), message.ReasonPositive))
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
			return fmt.Errorf("%s", message.EnvNameInvalid(name))
		}
		if trailingBackslashes(value)%2 == 1 {
			return fmt.Errorf("%s", message.EnvValueTrailingBackslash(name, value))
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
		return fmt.Errorf("%s", message.InterpreterCommandEmpty())
	}
	if strings.ContainsAny(it.Command, "\r\n") {
		return fmt.Errorf("%s", message.InterpreterCommandNewline(it.Command))
	}
	if len(it.Script) == 0 {
		return fmt.Errorf("%s", message.InterpreterScriptMissing())
	}
	norm := make(map[string]string, len(it.Script))
	seen := make(map[string]string, len(it.Script))
	for rawKey, rawVal := range it.Script {
		key := strings.TrimSpace(rawKey)
		val := strings.TrimSpace(rawVal)
		if key == "" {
			return fmt.Errorf("%s", message.InterpreterSuffixBlank())
		}
		if !strings.HasPrefix(key, ".") {
			return fmt.Errorf("%s", message.InterpreterSuffixNotDotted(key))
		}
		if val == "" {
			return fmt.Errorf("%s", message.InterpreterValueMissing(key))
		}
		if strings.ContainsAny(val, "\r\n") {
			return fmt.Errorf("%s", message.InterpreterValueNewline(key, val))
		}
		lower := strings.ToLower(key)
		if prev, dup := seen[lower]; dup {
			return fmt.Errorf("%s", message.InterpreterSuffixDuplicate(prev, key))
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

	// 这一族的话按「哪一种毛病」分：字与例子都在 message，这里只挑那一种。
	pathErr := func(kind message.PathProblemKind) error {
		return fmt.Errorf("%s", message.ConfigPathProblem(name, raw, kind, isWindows()))
	}

	if strings.HasPrefix(p, "~") {
		return "", pathErr(message.PathTilde)
	}

	if isWindows() {
		if filepath.IsAbs(p) {
			return p, nil
		}
		if strings.HasPrefix(p, "/") {
			return "", pathErr(message.PathNoDrive)
		}
		return "", pathErr(message.PathRelative)
	}

	if hasDrive {
		return "", pathErr(message.PathDriveOnUnix)
	}
	if strings.HasPrefix(p, "//") {
		return "", pathErr(message.PathUncOnUnix)
	}
	if filepath.IsAbs(p) {
		return p, nil
	}
	return "", pathErr(message.PathRelative)
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
