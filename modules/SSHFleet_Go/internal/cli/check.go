package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"sshfleet/internal/config"
)

// CheckConfigFiles 检查批量执行所需的规则文件是否齐全。
// 配置文件本体已在主干第 1 步加载（缺失即报错），此处只查两份规则文件。
func CheckConfigFiles() error {
	var missing []string
	for _, f := range []string{config.BuiltinPaths.DangerousKeywords, config.BuiltinPaths.ErrorKeywords} {
		if _, err := os.Stat(f); err != nil {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("配置文件缺失: %s", strings.Join(missing, ", "))
	}
	return nil
}

var loopKeywordRe = regexp.MustCompile(`^[^a-zA-Z]*([a-zA-Z]+)`)

// CheckArguments 参数合规性检查。23 条校验保持旧版结构与顺序，
// 错误统一返回给 main 打印。差异见 spec D40（-k 三态）、D29（脚本 CRLF
// 不再改写本地文件）、ADR-0002（-p 消歧义约束措辞）、D13（内联预检仅 IPv4）。
func CheckArguments(a *Args) error {
	// 执行模式互斥（手写校验）
	modeCount := 0
	for _, v := range []string{a.Command, a.Script, a.Upload, a.Download} {
		if v != "" {
			modeCount++
		}
	}
	if modeCount != 1 {
		return fmt.Errorf("执行模式参数：-c、-s、-u、-d 互斥，只能指定一个")
	}

	// -p
	if a.Path != "" {
		if a.Upload != "" {
			if !strings.HasPrefix(a.Path, "/") {
				return fmt.Errorf("上传模式：-p 参数指定的上传目录必须是绝对路径，当前值：%s", a.Path)
			}
			if !strings.HasSuffix(a.Path, "/") {
				// 消歧义约束（ADR-0002）：断言「这是目录」，排除把 -p 当目标文件名的误读
				return fmt.Errorf("上传模式：-p 必须是以 / 结尾的目录（-p 只表示放到哪个目录，工具不做远端重命名），当前值：%s", a.Path)
			}
		} else if a.Command != "" || a.Script != "" {
			return fmt.Errorf("-p 参数不能搭配 -c 或 -s 使用，请单独使用 -u 参数指定上传文件或目录后再使用 -c 或 -s")
		}
	}

	// -d
	if a.Download != "" {
		if a.Path == "" {
			return fmt.Errorf("-d 参数必须搭配 -p 参数使用")
		}
		if !strings.HasPrefix(a.Download, "/") {
			return fmt.Errorf("下载模式：-d 参数指定的远程路径必须是绝对路径，当前值：%s", a.Download)
		}
		if info, err := os.Stat(a.Path); err != nil || !info.IsDir() {
			return fmt.Errorf("下载模式：-p 参数指定的本地路径必须是已存在的目录，当前值：%s", a.Path)
		}
	}

	// -u
	if a.Upload != "" {
		if a.Path == "" {
			return fmt.Errorf("-u 参数必须搭配 -p 参数使用")
		}
		info, err := os.Lstat(a.Upload)
		if err != nil {
			return fmt.Errorf("-u 参数指定的上传文件或目录不存在：%s", a.Upload)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("-u 参数指定的上传文件或目录是符号链接，不能上传：%s", a.Upload)
		}
		if info.IsDir() && !dirHasRealFile(a.Upload) {
			return fmt.Errorf("-u 参数指定的上传目录及其所有子目录中都没有真正的文件（只有符号链接）：%s", a.Upload)
		}
	}

	// 执行身份：--sudo 与 --no-sudo 是一对开关，同时写就是自相矛盾
	if a.sudoFlag && a.noSudoFlag {
		return fmt.Errorf("--sudo 与 --no-sudo 只能给一个（两个都不给=按配置里的值）")
	}

	// -c
	if a.Command != "" {
		if strings.TrimSpace(a.Command) == "" {
			return fmt.Errorf("-c 参数不能为空，请提供要执行的命令")
		}
		if m := loopKeywordRe.FindStringSubmatch(strings.TrimSpace(a.Command)); m != nil {
			switch m[1] {
			case "for", "while", "until", "if", "case":
				return fmt.Errorf("-c 参数不兼容执行循环的命令，请使用脚本模式执行")
			}
		}
	}

	// -s
	if a.Script != "" {
		if info, err := os.Stat(a.Script); err == nil && info.IsDir() {
			return fmt.Errorf("-s 参数指定的路径是目录，不是脚本文件：%s", a.Script)
		}
		data, err := os.ReadFile(a.Script)
		if err != nil {
			return fmt.Errorf("-s 参数指定的脚本文件不可读：%s", a.Script)
		}
		if len(data) == 0 {
			return fmt.Errorf("-s 参数指定的脚本文件为空：%s", a.Script)
		}
		if !strings.HasSuffix(a.Script, ".sh") && !strings.HasSuffix(a.Script, ".py") {
			return fmt.Errorf("-s 参数指定的脚本文件扩展名必须是 .sh 或 .py，当前值：%s", a.Script)
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return fmt.Errorf("错误: %s 是二进制文件", a.Script)
		}
		if !utf8ValidExceptBOM(data) {
			return fmt.Errorf("错误: %s 不是UTF-8编码", a.Script)
		}
		// 旧版此处会把 CRLF 覆盖写回本地文件（名为 check 实有写副作用）；
		// spec D29：不再改写本地文件，改为上传时在内存内转换（M3）。
	}

	// -f
	if a.CsvFile != "" {
		if _, err := os.Stat(a.CsvFile); err != nil {
			// 文件不存在：首字段是 IPv4 才视为内联清单文本（D13），否则报路径不存在
			firstField := strings.TrimSpace(strings.Split(a.CsvFile, ",")[0])
			addr, err := netip.ParseAddr(firstField)
			if err != nil || !addr.Is4() {
				return fmt.Errorf("-f 参数指定的文件不存在：%s\n提示：如需内联传入节点，请以 IP 开头（如 192.168.1.1,22,root,密码）", a.CsvFile)
			}
			a.FIsInline = true
		} else {
			a.FIsInline = false
			data, err := os.ReadFile(a.CsvFile)
			if err != nil {
				return fmt.Errorf("-f 参数指定的 CSV 文件不可读：%s", a.CsvFile)
			}
			if len(data) == 0 {
				return fmt.Errorf("-f 参数指定的 CSV 文件为空：%s", a.CsvFile)
			}
			if bytes.IndexByte(data[:min(1024, len(data))], 0) >= 0 {
				return fmt.Errorf("错误: %s 是二进制文件", a.CsvFile)
			}
		}
	}

	// -n
	if err := checkPositiveInt(a.numberRaw, a.Number, a.numberInvalid, "-n", "并发连接数"); err != nil {
		return err
	}

	// -k 只表示"用密钥登录"，没有路径参数可查；私钥是否可用交给凭据预检（M2）

	// -t
	if err := checkPositiveInt(a.timeoutRaw, a.Timeout, a.timeoutInvalid, "-t", "命令或传输超时时间"); err != nil {
		return err
	}

	// -T
	if err := checkPositiveInt(a.connectTimeoutRaw, a.ConnectTimeout, a.connectTimeoutInvalid, "-T", "连接超时时间"); err != nil {
		return err
	}

	return nil
}

// keyTools 本次给出的密钥管理命令名（`--gen-key` / `--key-status` / `--convert-secret`）。
func keyTools(a *Args) []string {
	var names []string
	if a.GenKey {
		names = append(names, "--gen-key")
	}
	if a.KeyStatus {
		names = append(names, "--key-status")
	}
	if a.ConvertPassword != "" {
		names = append(names, "--convert-secret")
	}
	return names
}

// batchParamsGiven 本次给出的批量执行参数（模式、清单、目标路径、执行参数、-k）。
// `--yes` 不算：它对密钥管理命令也有意义（跳过覆盖确认），且不指向任何节点。
func batchParamsGiven(a *Args) []string {
	var given []string
	for _, m := range [][2]string{{"-c", a.Command}, {"-s", a.Script}, {"-u", a.Upload}, {"-d", a.Download}} {
		if m[1] != "" {
			given = append(given, m[0])
		}
	}
	if a.CsvFile != "" {
		given = append(given, "-f")
	}
	if a.Path != "" {
		given = append(given, "-p")
	}
	if a.numberRaw != "" {
		given = append(given, "-n")
	}
	if a.Remark != "" {
		given = append(given, "-r")
	}
	if a.timeoutRaw != "" {
		given = append(given, "-t")
	}
	if a.connectTimeoutRaw != "" {
		given = append(given, "-T")
	}
	if a.Key {
		given = append(given, "-k")
	}
	if a.NoBash {
		given = append(given, "--no-bash")
	}
	if a.sudoFlag {
		given = append(given, "--sudo")
	}
	if a.noSudoFlag {
		given = append(given, "--no-sudo")
	}
	return given
}

// CheckKeyToolExclusivity 密钥管理命令与批量执行参数不能同时给：检查通过就意味着
// 它要做的事一定会做，不允许参数被悄悄丢掉。多个密钥管理命令同时给也在这里拦。
//
// 这一步跑在工具模式分流之前（那时还没走到批量参数检查），所以单独成一个入口。
func CheckKeyToolExclusivity(a *Args) error {
	tools := keyTools(a)
	if len(tools) == 0 {
		return nil
	}
	if len(tools) > 1 {
		return fmt.Errorf("%s 只能给一个——它们是三个独立命令，一次做一件事", strings.Join(tools, "、"))
	}
	if given := batchParamsGiven(a); len(given) > 0 {
		return fmt.Errorf(
			"%s 不能和批量执行参数一起用：同时给了 %s\n"+
				"提示：分成两次执行——先跑密钥管理命令，再跑批量执行", tools[0], strings.Join(given, "、"))
	}
	return nil
}

// checkPositiveInt 正整数校验。旧版语义：缺省（含显式 0）不校验；
// 非法字符串与负数报「参数格式错误」。
func checkPositiveInt(raw string, val int, invalid bool, flag, what string) error {
	if raw == "" || raw == "0" {
		return nil
	}
	if invalid || val <= 0 {
		return fmt.Errorf("%s 参数格式错误，%s必须是正整数，当前值：%s", flag, what, raw)
	}
	return nil
}

// dirHasRealFile 目录树中是否存在真实文件（非符号链接）。
func dirHasRealFile(root string) bool {
	found := false
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return fs.SkipAll
		}
		if d.Type().IsRegular() {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// utf8ValidExceptBOM 判断内容为合法 UTF-8（允许带 BOM，对位旧 check_script_file）。
func utf8ValidExceptBOM(data []byte) bool {
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		data = data[3:]
	}
	return utf8.Valid(data)
}
