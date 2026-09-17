// Package config 承载主干第 1 步：加载 ./config/SSHFleet.conf（TOML）。
//
// 字段基准 = 旧 YAML 配置；与旧实现不同的部分见 spec D6 / D28 / D31：
//   - 删 paths.exe 段；keywords/logs/files 三段两层并入单段 [paths]
//   - password_security 为 int，仅允许 1/2/3
//   - **所有字段必填、全工具不写死任何默认值**（用户 2026-09-14 裁定）：缺字段与非法取值都在此报错
//   - 未知字段零容忍：解码后取未识别键，报错列出具体键名
//   - 路径解析一条规则：去首尾空白 → ~ 展开 → 绝对则原样 → 相对则拼 secret_dir
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Account struct {
	Port             int    `toml:"port"`
	User             string `toml:"user"`
	PasswordSecurity int    `toml:"password_security"`
	SecretDir        string `toml:"secret_dir"`
	Password         string `toml:"password"`
	Key              string `toml:"key"`
	KeyPassphrase    string `toml:"key_passphrase"`
}

type Execution struct {
	Mode            string `toml:"mode"`
	TimeoutConnect  int    `toml:"timeout_connect"`
	TimeoutExecute  int    `toml:"timeout_execute"`
	TimeoutTransfer int    `toml:"timeout_transfer"`
}

type Enable struct {
	OutputToXlsx     bool `toml:"output_to_xlsx"`
	ResultsToXlsx    bool `toml:"results_to_xlsx"`
	ShowCategoryTips bool `toml:"show_category_tips"`
}

// Paths：旧 paths.keywords / paths.logs / paths.files 三段并为一层（spec D6）。
type Paths struct {
	ErrorKeywords     string `toml:"error_keywords"`
	DangerousKeywords string `toml:"dangerous_keywords"`
	Historys          string `toml:"historys"`
	Tool              string `toml:"tool"`
	Exec              string `toml:"exec"`
	Asset             string `toml:"asset"`
	Output            string `toml:"output"`
	OutputXlsx        string `toml:"output_xlsx"`
	Report            string `toml:"report"`
	ResultsXlsx       string `toml:"results_xlsx"`
}

type UploadConcurrencyThresholds struct {
	SmallFile         int `toml:"small_file"`
	LargeFile         int `toml:"large_file"`
	MediumConcurrency int `toml:"medium_concurrency"`
}

type Upload struct {
	ConcurrencyThresholds UploadConcurrencyThresholds `toml:"concurrency_thresholds"`
}

type Config struct {
	Account   Account   `toml:"account"`
	Execution Execution `toml:"execution"`
	Enable    Enable    `toml:"enable"`
	Paths     Paths     `toml:"paths"`
	Upload    Upload    `toml:"upload"`
}

// Load 读取并校验配置文件，返回可直接使用的配置。
func Load(path string) (*Config, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("配置文件 %s 不存在", path)
	}

	var cfg Config
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, fmt.Errorf("解析 TOML 失败：%v", err)
	}

	// 未知字段零容忍：列出具体键名（对位旧 pydantic extra="forbid"）
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, k := range undecoded {
			keys = append(keys, k.String())
		}
		return nil, fmt.Errorf("配置包含未识别字段：%s", strings.Join(keys, ", "))
	}

	if err := validate(&cfg, md); err != nil {
		return nil, err
	}
	if err := resolveCredentialPaths(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// validate 配置预检查：必填性（所有字段） + 取值合规性。
// 用户 2026-09-14 裁定：全工具不写死任何默认值——预检查通过即代表配置自足，
// 运行期一律取配置里的值。缺项一次列全，取值逐项校验并指出非法值。
func validate(cfg *Config, md toml.MetaData) error {
	required := []struct {
		key string
		val string
	}{
		{"account.port", definedInt(md, "account", "port")},
		{"account.user", definedStr(md, "account", "user")},
		{"account.password_security", definedInt(md, "account", "password_security")},
		{"account.secret_dir", definedStr(md, "account", "secret_dir")},
		{"account.password", definedStr(md, "account", "password")},
		{"account.key", definedStr(md, "account", "key")},
		{"account.key_passphrase", definedStr(md, "account", "key_passphrase")},
		{"execution.mode", definedStr(md, "execution", "mode")},
		{"execution.timeout_connect", definedInt(md, "execution", "timeout_connect")},
		{"execution.timeout_execute", definedInt(md, "execution", "timeout_execute")},
		{"execution.timeout_transfer", definedInt(md, "execution", "timeout_transfer")},
		{"enable.output_to_xlsx", definedStr(md, "enable", "output_to_xlsx")},
		{"enable.results_to_xlsx", definedStr(md, "enable", "results_to_xlsx")},
		{"enable.show_category_tips", definedStr(md, "enable", "show_category_tips")},
		{"paths.error_keywords", definedStr(md, "paths", "error_keywords")},
		{"paths.dangerous_keywords", definedStr(md, "paths", "dangerous_keywords")},
		{"paths.historys", definedStr(md, "paths", "historys")},
		{"paths.tool", definedStr(md, "paths", "tool")},
		{"paths.exec", definedStr(md, "paths", "exec")},
		{"paths.asset", definedStr(md, "paths", "asset")},
		{"paths.output", definedStr(md, "paths", "output")},
		{"paths.output_xlsx", definedStr(md, "paths", "output_xlsx")},
		{"paths.report", definedStr(md, "paths", "report")},
		{"paths.results_xlsx", definedStr(md, "paths", "results_xlsx")},
		{"upload.concurrency_thresholds.small_file", definedInt(md, "upload", "concurrency_thresholds", "small_file")},
		{"upload.concurrency_thresholds.large_file", definedInt(md, "upload", "concurrency_thresholds", "large_file")},
		{"upload.concurrency_thresholds.medium_concurrency", definedInt(md, "upload", "concurrency_thresholds", "medium_concurrency")},
	}
	var missing []string
	for _, r := range required {
		if r.val == "" {
			missing = append(missing, r.key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("配置缺少必填字段：%s", strings.Join(missing, ", "))
	}

	// 取值合规性（预检查通过即配置自足，运行期不再兜底）
	if cfg.Account.Port < 1 || cfg.Account.Port > 65535 {
		return fmt.Errorf("account.port 取值非法：%d（须为 1-65535）", cfg.Account.Port)
	}
	if cfg.Account.PasswordSecurity != 1 && cfg.Account.PasswordSecurity != 2 && cfg.Account.PasswordSecurity != 3 {
		return fmt.Errorf("account.password_security 取值非法：%d（仅支持 1/2/3：1=明文，2=base64，3=加密）", cfg.Account.PasswordSecurity)
	}
	if cfg.Execution.Mode != "direct" && cfg.Execution.Mode != "sudo" {
		return fmt.Errorf("execution.mode 取值非法：%s（仅支持 direct=登录用户 / sudo=root）", cfg.Execution.Mode)
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
	th := cfg.Upload.ConcurrencyThresholds
	if th.SmallFile < 1 {
		return fmt.Errorf("upload.concurrency_thresholds.small_file 取值非法：%d（须为正整数，单位字节）", th.SmallFile)
	}
	if th.LargeFile < 1 {
		return fmt.Errorf("upload.concurrency_thresholds.large_file 取值非法：%d（须为正整数，单位字节）", th.LargeFile)
	}
	if th.LargeFile < th.SmallFile {
		return fmt.Errorf("upload.concurrency_thresholds 取值非法：large_file(%d) 不能小于 small_file(%d)", th.LargeFile, th.SmallFile)
	}
	if th.MediumConcurrency < 1 {
		return fmt.Errorf("upload.concurrency_thresholds.medium_concurrency 取值非法：%d（须为正整数）", th.MediumConcurrency)
	}
	return nil
}

func definedStr(md toml.MetaData, path ...string) string {
	if md.IsDefined(path...) {
		return "1"
	}
	return ""
}

func definedInt(md toml.MetaData, path ...string) string { return definedStr(md, path...) }

// resolveCredentialPaths 解析配置内三个凭据字段（spec D31）：
// 去首尾空白 → ~ 展开 → 绝对则原样 → 相对则拼 secret_dir。
// secret_dir 未配置且字段为相对路径 → 明确报错，给出两条出路。
// 与旧实现的差异：取消 secret_dir 字面量 none 的宽容（TOML 中 "" 即未配置）。
func resolveCredentialPaths(cfg *Config) error {
	secretDir := strings.TrimSpace(cfg.Account.SecretDir)
	if secretDir != "" {
		secretDir = expandHome(secretDir)
		cfg.Account.SecretDir = secretDir
	}

	resolve := func(field, raw string) (string, error) {
		p := strings.TrimSpace(raw)
		if p == "" {
			return "", nil
		}
		if strings.HasPrefix(p, "~") {
			p = expandHome(p)
		}
		if filepath.IsAbs(p) {
			return p, nil
		}
		if secretDir == "" {
			return "", fmt.Errorf(
				"account.%s 为相对路径 '%s'，但 account.secret_dir 未配置，无法拼接\n出路：① 改写绝对路径 ② 在配置中设置 account.secret_dir", field, strings.TrimSpace(raw))
		}
		return filepath.Join(secretDir, p), nil
	}

	// password 为必填字段，Load 出此函数前已确认存在；key / key_passphrase 允许为空。
	var err error
	cfg.Account.Password, err = resolve("password", cfg.Account.Password)
	if err == nil {
		cfg.Account.Key, err = resolve("key", cfg.Account.Key)
	}
	if err == nil {
		cfg.Account.KeyPassphrase, err = resolve("key_passphrase", cfg.Account.KeyPassphrase)
	}
	return err
}

// expandHome 把前导 ~ 展开为用户主目录。
func expandHome(p string) string {
	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}
