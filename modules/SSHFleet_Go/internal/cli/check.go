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

	"sshfleet/internal/common"
	"sshfleet/internal/config"
)

// CheckConfigFiles 检查批量执行所需的规则文件是否齐全。
// 配置文件本体已在主干第 1 步加载（缺失即报错），此处只查三份规则文件。
func CheckConfigFiles() error {
	var missing []string
	for _, f := range []string{
		config.BuiltinPaths.DangerousKeywords,
		config.BuiltinPaths.ErrorKeywords,
		config.BuiltinPaths.PasswdKeywords,
	} {
		if _, err := os.Stat(f); err != nil {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		// 标签不能用「配置文件缺失」——查的是规则文件，用户会跑去翻配置。
		// 半角冒号也一并改全角（同一份输出里的标点要一致）。
		return fmt.Errorf("规则文件缺失：%s\n提示：这几份文件随发布包提供，不要删；从压缩包里重新解压一份覆盖回 config/ 即可",
			strings.Join(missing, "、"))
	}
	return nil
}

var loopKeywordRe = regexp.MustCompile(`^[^a-zA-Z]*([a-zA-Z]+)`)

// CheckArguments 参数合规性检查。23 条校验保持旧版结构与顺序，
// 错误统一返回给 main 打印。差异见 spec D40（-k 三态）、D29（脚本 CRLF
// 不再改写本地文件）、ADR-0002（-p 消歧义约束措辞）、D13（内联预检仅 IPv4）。
func CheckArguments(a *Args) error {
	// 执行模式互斥（手写校验）。零个与多个分开说——"互斥、只能指定一个"
	// 对一个都没给是误导：他要做的是"选一个"，不是"删掉多余的"。
	modeCount := 0
	for _, v := range []string{a.Command, a.Script, a.Upload, a.Download, a.ChangePassword} {
		if v != "" {
			modeCount++
		}
	}
	if modeCount == 0 {
		return fmt.Errorf(
			"执行模式参数：-c、-s、-u、-d、--change-password 互斥，只能指定一个\n" +
				"提示：你没有指定任何模式，请从下面五个里选一个：\n" +
				"      -c 执行命令   -s 执行脚本   -u 上传文件   -d 下载文件   --change-password 批量改密")
	}
	if modeCount > 1 {
		return fmt.Errorf("执行模式参数：-c、-s、-u、-d、--change-password 互斥，只能指定一个\n提示：一次只能做一件事，请只保留一个模式")
	}

	// --change-password：互斥与取值（放这里，早于下面那些"只为某个模式服务"的校验）
	if err := checkPasswdChange(a); err != nil {
		return err
	}

	// -p
	if a.Path != "" {
		if a.Upload != "" {
			if !strings.HasPrefix(a.Path, "/") {
				return fmt.Errorf("上传模式：-p 参数指定的上传目录必须是绝对路径，当前值：%s\n提示：服务器上的目录要从 / 写起，例如 -p /opt/app/", a.Path)
			}
			if !strings.HasSuffix(a.Path, "/") {
				// 消歧义约束（ADR-0002）：断言「这是目录」，排除把 -p 当目标文件名的误读
				return fmt.Errorf("上传模式：-p 必须是以 / 结尾的目录，当前值：%s\n提示：-p 只表示\"放到哪个目录\"，工具不会替你改文件名，所以要以 / 收尾", a.Path)
			}
		} else if a.Command != "" || a.Script != "" {
			return fmt.Errorf("-p 参数不能搭配 -c 或 -s 使用\n提示：只有上传、下载才用 -p；命令与脚本模式不需要它")
		}
	}

	// -d
	if a.Download != "" {
		if a.Path == "" {
			return fmt.Errorf("-d 参数必须搭配 -p 参数使用\n提示：-p 写文件要存到本机的哪个目录，例如 -p ./logs/")
		}
		if !strings.HasPrefix(a.Download, "/") {
			return fmt.Errorf("下载模式：-d 参数指定的远程路径必须是绝对路径，当前值：%s\n提示：服务器上的路径要从 / 写起，例如 /var/log/app", a.Download)
		}
		if info, err := os.Stat(a.Path); err != nil || !info.IsDir() {
			return fmt.Errorf("下载模式：-p 参数指定的本地路径必须是已存在的目录，当前值：%s\n提示：这个目录要在本机上先建好，工具不会替你建", a.Path)
		}
	}

	// -u
	if a.Upload != "" {
		if a.Path == "" {
			return fmt.Errorf("-u 参数必须搭配 -p 参数使用\n提示：-p 写文件要放到服务器上的哪个目录，例如 -p /opt/app/")
		}
		info, err := os.Lstat(a.Upload)
		if err != nil {
			return fmt.Errorf("-u 参数指定的上传文件或目录不存在：%s\n提示：-u 后面要写本机上真实存在的文件或目录", a.Upload)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("-u 参数指定的上传文件或目录是符号链接，不能上传：%s\n提示：请直接写它指向的真实文件或目录", a.Upload)
		}
		if info.IsDir() && !dirHasRealFile(a.Upload) {
			return fmt.Errorf("-u 参数指定的上传目录及其所有子目录中都没有真正的文件（只有符号链接）：%s\n提示：请改上传真实文件所在的目录", a.Upload)
		}
	}

	// 执行身份：--sudo 与 --no-sudo 是一对开关，同时写就是自相矛盾
	if a.sudoFlag && a.noSudoFlag {
		return fmt.Errorf("--sudo 与 --no-sudo 只能给一个（两个都不给=按配置里的值）")
	}

	// -c
	if a.Command != "" {
		if strings.TrimSpace(a.Command) == "" {
			return fmt.Errorf("-c 参数不能为空，请提供要执行的命令\n提示：-c 后面跟命令原文，例如 -c \"systemctl status sshd\"")
		}
		if m := loopKeywordRe.FindStringSubmatch(strings.TrimSpace(a.Command)); m != nil {
			switch m[1] {
			case "for", "while", "until", "if", "case":
				return fmt.Errorf("-c 参数不兼容执行循环的命令，请使用脚本模式执行")
			}
		}
	}

	// -s
	var scriptText []byte // -a 的长度检查要用同一份正文算下发行，不再读一次文件
	if a.Script != "" {
		if info, err := os.Stat(a.Script); err == nil && info.IsDir() {
			return fmt.Errorf("-s 参数指定的路径是目录，不是脚本文件：%s\n提示：请指向一个 .sh 或 .py 文件", a.Script)
		}
		data, err := os.ReadFile(a.Script)
		if err != nil {
			return fmt.Errorf("-s 参数指定的脚本文件不可读：%s\n提示：请检查文件是否存在、当前用户有没有读权限", a.Script)
		}
		if len(data) == 0 {
			return fmt.Errorf("-s 参数指定的脚本文件为空：%s\n提示：请先写入内容再执行", a.Script)
		}
		if !strings.HasSuffix(a.Script, ".sh") && !strings.HasSuffix(a.Script, ".py") {
			return fmt.Errorf("-s 参数指定的脚本文件扩展名必须是 .sh 或 .py，当前值：%s\n提示：请改后缀，或换一个脚本文件", a.Script)
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return fmt.Errorf("%s 是二进制文件\n提示：请换一个文本格式的脚本", a.Script)
		}
		if !utf8ValidExceptBOM(data) {
			return fmt.Errorf("%s 不是 UTF-8 编码\n提示：请把脚本转成 UTF-8 后再执行", a.Script)
		}
		// 脚本内容里的 CRLF 不改写本地文件：上传时在内存内转换。
		scriptText = data
	}

	// -a（代填）：来源判定、解析、门控、互斥与长度检查都在这里做完
	if err := checkAnswer(a, scriptText); err != nil {
		return err
	}

	// -f：来源（文件 / 内联文本）已由 Parse 判定，这里按来源分流——
	// 内联文本要看首字段像不像节点，文件要能读、且不是二进制。
	if a.CsvFile != "" {
		if a.FIsInline {
			// 首字段是 IPv4 才算内联清单文本，否则多半是把路径打错了
			firstField := strings.TrimSpace(strings.Split(a.CsvFile, ",")[0])
			addr, err := netip.ParseAddr(firstField)
			if err != nil || !addr.Is4() {
				return fmt.Errorf("-f 参数指定的文件不存在：%s\n提示：要临时传几台机器，可以直接写一行以 IP 开头的节点信息，例如 192.168.1.1,22,root,密码", a.CsvFile)
			}
		} else {
			data, err := os.ReadFile(a.CsvFile)
			if err != nil {
				return fmt.Errorf("-f 参数指定的 CSV 文件不可读：%s\n提示：请检查文件是否存在、当前用户有没有读权限", a.CsvFile)
			}
			if len(data) == 0 {
				return fmt.Errorf("-f 参数指定的 CSV 文件为空：%s\n提示：一行写一台机器，只写 IP 就能跑（端口、用户名、密码走配置）", a.CsvFile)
			}
			if common.IsBinaryContent(data) {
				return fmt.Errorf("%s 是二进制文件\n提示：清单要是文本文件（CSV）", a.CsvFile)
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
	if a.ConvertSecret != "" {
		names = append(names, "--convert-secret")
	}
	return names
}

// batchParamsGiven 本次给出的批量执行参数（模式、清单、目标路径、执行参数、-k）。
// `--yes` 不算：它对密钥管理命令也有意义（跳过覆盖确认），且不指向任何节点。
func batchParamsGiven(a *Args) []string {
	var given []string
	for _, m := range [][2]string{{"-c", a.Command}, {"-s", a.Script}, {"-u", a.Upload}, {"-d", a.Download}, {"--change-password", a.ChangePassword}} {
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
		return fmt.Errorf("%s 参数格式错误，%s必须是正整数，当前值：%s\n提示：给一个正整数（单位见该参数的说明）", flag, what, raw)
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
