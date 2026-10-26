package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// CheckArguments 选项/参数合规性检查。23 条校验保持旧版结构与顺序，
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
			"执行模式选项：-c、-s、-u、-d、--change-password 互斥，只能指定一个\n" +
				"提示：你没有指定任何模式，请从下面五个里选一个：\n" +
				"      -c 执行命令   -s 执行脚本   -u 上传文件   -d 下载文件   --change-password 批量改密")
	}
	if modeCount > 1 {
		return fmt.Errorf("执行模式选项：-c、-s、-u、-d、--change-password 互斥，只能指定一个\n提示：一次只能做一件事，请只保留一个模式")
	}

	// --change-password：互斥与取值（放这里，早于下面那些"只为某个模式服务"的校验）
	if err := checkPasswdChange(a); err != nil {
		return err
	}

	// -p
	if a.Path != "" {
		if a.Upload != "" {
			// 上传时 -p 是**服务器上的目录**：按远端语义检查，与本机解析无关
			if err := checkRemotePath("-p", "上传目录", a.Path); err != nil {
				return err
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
		// -d 恒为远端路径：同一套远端形态检查
		if err := checkRemotePath("-d", "远程路径", a.Download); err != nil {
			return err
		}
		if info, err := os.Stat(a.Path); err != nil {
			return fmt.Errorf("下载模式：-p 参数指定的本地目录读不到：%s\n原因：%s\n提示：这个目录要在本机上先建好，工具不会替你建", a.Path, systemReason(err))
		} else if !info.IsDir() {
			// 不是目录时去要一句系统原文：本机自己的措辞随平台变（Windows 与 POSIX 不同），
			// 我们不去编一句「not a directory」冒充它。
			_, dirErr := os.ReadDir(a.Path)
			return fmt.Errorf("下载模式：-p 参数指定的本地路径不是目录：%s\n原因：%s\n提示：这个目录要在本机上先建好，工具不会替你建", a.Path, systemReason(dirErr))
		}
	}

	// -u
	if a.Upload != "" {
		if a.Path == "" {
			return fmt.Errorf("-u 参数必须搭配 -p 参数使用\n提示：-p 写文件要放到服务器上的哪个目录，例如 -p /opt/app/")
		}
		info, err := os.Lstat(a.Upload)
		if err != nil {
			return fmt.Errorf("-u 参数指定的上传文件或目录读不到：%s\n原因：%s\n提示：-u 后面要写本机上真实存在的文件或目录", a.Upload, systemReason(err))
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
	}

	// -s
	var scriptText []byte // -a 的长度检查要用同一份正文算下发行，不再读一次文件
	if a.Script != "" {
		if info, err := os.Stat(a.Script); err == nil && info.IsDir() {
			return fmt.Errorf("-s 参数指定的路径是目录，不是脚本文件：%s\n提示：请指向一个脚本文件", a.Script)
		}
		data, err := os.ReadFile(a.Script)
		if err != nil {
			return fmt.Errorf("-s 参数指定的脚本文件读不到：%s\n原因：%s\n提示：请检查文件是否存在、当前用户有没有读权限", a.Script, systemReason(err))
		}
		if len(data) == 0 {
			return fmt.Errorf("-s 参数指定的脚本文件为空：%s\n提示：请先写入内容再执行", a.Script)
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return fmt.Errorf("-s 参数指定的脚本是二进制文件：%s\n提示：请换一个文本格式的脚本", a.Script)
		}
		if !utf8ValidExceptBOM(data) {
			return fmt.Errorf("-s 参数指定的脚本不是 UTF-8 编码：%s\n提示：请把脚本转成 UTF-8 后再执行", a.Script)
		}
		// 脚本内容里的 CRLF 不改写本地文件：上传时在内存内转换。
		scriptText = data
	}

	// -a（代填）：来源判定、解析、门控、互斥与长度检查都在这里做完
	if err := checkAnswer(a, scriptText); err != nil {
		return err
	}
	// -a 的解释器范围：正文要作"一段程序文本"交给解释器，各家参数不同（见 ssh.ProgramTextArg）
	if err := checkAnswerInterpreter(a); err != nil {
		return err
	}

	// -f：取值判定与 -a 共用 resolveValue（先原样按路径找、不是文件再看能不能当一条
	// 记录读、都不是报同形的一句话），来源在这里落回 FIsInline——装载与归档要用它。
	// 命中文件之后才查这个文件读不读得了、空不空、是不是二进制。
	if a.CsvFile != "" {
		src, err := resolveValue(a.CsvFile, nodeListForm)
		if err != nil {
			return err
		}
		a.FIsInline = !src.IsFile
		if src.IsFile {
			data, err := os.ReadFile(src.Path)
			if err != nil {
				return fmt.Errorf("-f 参数指定的 CSV 文件读不到：%s\n原因：%s\n提示：请检查文件是否存在、当前用户有没有读权限", src.Path, systemReason(err))
			}
			if len(data) == 0 {
				return fmt.Errorf("-f 参数指定的 CSV 文件为空：%s\n提示：一行写一台机器，只写 IP 就能跑（端口、用户名、密码走配置）", src.Path)
			}
			if common.IsBinaryContent(data) {
				return fmt.Errorf("-f 参数指定的清单是二进制文件：%s\n提示：清单要是文本文件（CSV）", src.Path)
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
	if a.NoShell {
		given = append(given, "--no-shell")
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
			"%s 不能和批量执行选项一起用：同时给了 %s\n"+
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
		return fmt.Errorf("%s 参数格式错误，%s必须是正整数，当前值：%s\n提示：给一个正整数（单位见该选项的说明）", flag, what, raw)
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

// checkRemotePath 远端路径的形态检查：**不是本机解析**，远端那条路径本机解析不了。
//
// 它只回答一件事——交上来的这条路径，像不像「使用者本来就想写的那条完整路径」。
// 四种不像的情形都拦下来，让错误停在参数阶段，而不是等远端回一句看不懂的失败：
//
//   - 不以 `/` 开头：不是绝对路径，远端拿它当相对路径解释，落点全凭登录目录；
//   - 含空白：漏引号的唯一信号（被空格切开的半截路径也长这样）；
//   - 含 `//`：多半是拼接时多写了一段分隔符；
//   - 含 `.` / `..` 段：路径没写干净，远端要不要压平各家不同，不猜。
//
// 这是工具自己的规矩，不是替远端解析路径——值本身一个字节都不改。
func checkRemotePath(flag, what, p string) error {
	reason := ""
	hint := fmt.Sprintf("%s 收的是服务器上的%s，请检查是否漏写引号或参数拼写有误", flag, what)
	// 空白先判：` /var/log ` 这种形态两头都有问题，先报「有空白」才指得准。
	switch {
	case strings.ContainsAny(p, " \t"):
		reason = "不能含空格或制表符，当前值："
	case !strings.HasPrefix(p, "/"):
		reason = "必须是绝对路径，当前值："
	case strings.Contains(p[1:], "//"):
		reason = "不能含连续的 //，当前值："
	case hasDotSegment(p):
		reason = "不能含 . 或 .. 这种段，当前值："
	default:
		return nil
	}
	if reason == "必须是绝对路径，当前值：" {
		return fmt.Errorf("%s 参数指定的%s必须是绝对路径，当前值：%s\n提示：服务器上的路径要从 / 写起，例如 /opt/app/", flag, what, p)
	}
	return fmt.Errorf("%s 参数指定的%s不干净：%s%s\n提示：%s", flag, what, reason, p, hint)
}

// hasDotSegment 路径里是否含 `.` 或 `..` 段（按 POSIX 的 / 切分）。
func hasDotSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}

// utf8ValidExceptBOM 判断内容为合法 UTF-8（允许带 BOM，对位旧 check_script_file）。
func utf8ValidExceptBOM(data []byte) bool {
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		data = data[3:]
	}
	return utf8.Valid(data)
}

// systemReason 取系统给的那一段原因，去掉前面重复的路径。
//
// 报错第一行已经点过当前值了，这里只留错因本身：`open /x/y: no such file or directory`
// 取成 `no such file or directory`。两种形态都剥——*fs.PathError（stat / open / read）
// 与 *os.LinkError（rename 一类）。
func systemReason(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Err.Error()
	}
	return err.Error()
}
