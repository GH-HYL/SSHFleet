// Package cli 承载主干第 3 步（解析命令行）与第 5 步的合规检查部分。
//
// 与旧实现的差异（spec D24 / D26 / D40）：平级选项维持原样，不引子命令、
// 不引 cobra；互斥手写校验、提示文案自撰；-k 三态语义准确（见 KeyMode）。
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"sshfleet/internal/config"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/pflag"
)

// ErrHelp 表示已打印帮助、应以 0 退出（无任何参数或 -h/-–help）。
var ErrHelp = errors.New("help printed")

// keyModeSentinel 裸 -k 的哨兵值。只在 KeyMode() 一处解读，其余代码不得比较它。
const keyModeSentinel = "default"

// KeyMode 密钥三态（CONTEXT.md「密钥三态」）。
type KeyMode int

const (
	KeyModeOff       KeyMode = iota // 未指定 -k，强制空密钥
	KeyModeDefault                  // 仅 -k：逐节点按「清单 > 配置」解析密钥
	KeyModeUniversal                // -k 带路径：所有节点统一使用命令行私钥
)

// Args 是主干各步骤共用的命令行参数载体（纯数据 + 三态解读）。
type Args struct {
	Command         string // -c
	Script          string // -s
	Upload          string // -u
	Download        string // -d
	CsvFile         string // -f
	Path            string // -p
	Mode            string // -m
	Timeout         int    // -t（缺省时按模式取配置默认）
	ConnectTimeout  int    // -T
	Number          int    // -n
	Remark          string // -r
	NoBash          bool   // --nobash
	Disinteractive  bool   // --disinteractive
	Key             string // -k（含哨兵值）
	GenKey          bool   // --gen-key
	ConvertPassword string // --convert-password
	FIsInline       bool   // -f 为内联清单（由 CheckArguments 判定）

	// -k 是否在命令行出现（三态之 off 与 default 的分界）
	keyChanged bool

	// -t / -T / -n 的原始输入：非法值不在解析期报错，留给 CheckArguments
	// 按旧版口径报「参数格式错误」。
	timeoutRaw, connectTimeoutRaw, numberRaw             string
	timeoutInvalid, connectTimeoutInvalid, numberInvalid bool
}

// KeyMode 返回本次运行的密钥三态。
func (a *Args) KeyMode() KeyMode {
	if !a.keyChanged {
		return KeyModeOff
	}
	if a.Key == keyModeSentinel {
		return KeyModeDefault
	}
	return KeyModeUniversal
}

// Parse 解析命令行并补默认值。raw 是 os.Args[1:]，version 是入口定义的版本号（帮助显示用）。
func Parse(cfg *config.Config, version string, raw []string) (*Args, error) {
	fs := pflag.NewFlagSet("SSHFleet", pflag.ContinueOnError)
	fs.SetOutput(io.Discard) // 解析错误与提示全部自撰，不走 pflag 默认输出

	var a Args
	fs.StringVarP(&a.Command, "command", "c", "", "远程在多台服务器上执行一条命令")
	fs.StringVarP(&a.Script, "script", "s", "", "远程在多台服务器上执行一个本地脚本")
	fs.StringVarP(&a.Upload, "upload", "u", "", "把本地文件或目录传到服务器")
	fs.StringVarP(&a.Download, "download", "d", "", "从服务器下载文件或目录到本地")
	fs.StringVarP(&a.CsvFile, "file", "f", "", "节点清单：CSV 文件路径，或内联一行节点信息")
	fs.StringVarP(&a.Path, "path", "p", "", "目标路径：上传到服务器的目录 / 下载到的本地目录")
	fs.StringVarP(&a.Mode, "mode", "m", "", "执行身份: direct=登录用户, sudo=root")
	fs.StringVarP(&a.timeoutRaw, "timeout", "t", "", "单台执行或传输的超时时间（秒）")
	fs.StringVarP(&a.connectTimeoutRaw, "connect-timeout", "T", "", "连接每台服务器的超时时间（秒）")
	fs.StringVarP(&a.numberRaw, "number", "n", "", "并发数：同时操作几台服务器")
	fs.StringVarP(&a.Remark, "remark", "r", "", "本次任务的名称（历史记录文件夹后缀）")
	fs.BoolVar(&a.NoBash, "nobash", false, "命令模式专用: 不套一层 bash 环境")
	fs.BoolVar(&a.Disinteractive, "disinteractive", false, "跳过所有确认提示直接执行")
	fs.StringVarP(&a.Key, "key", "k", "", "不指定=纯密码; 仅 -k=清单/配置默认密钥; -k 路径=统一私钥")
	fs.BoolVar(&a.GenKey, "gen-key", false, "生成随机主密钥并持久化到 SSHFLEET_KEY")
	fs.StringVar(&a.ConvertPassword, "convert-password", "", "转换凭据文件（跟目标文件路径）")

	// 未提供任何参数：打印帮助后以 0 退出（与旧版一致，发生在配置加载之后）
	if len(raw) == 0 {
		Usage(cfg, version)
		return nil, ErrHelp
	}

	if err := fs.Parse(normalizeKeyFlag(raw)); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			Usage(cfg, version)
			return nil, ErrHelp
		}
		return nil, fmt.Errorf("%v（使用 -h 查看帮助）", err)
	}
	if kf := fs.Lookup("key"); kf != nil {
		a.keyChanged = kf.Changed
	}

	// 多出来的位置参数：一律报错。
	// 常见来源三类——① 通配符被终端展开（`-u /x/abc/*` 会变成多个路径，而 -u 只吃第一个，
	// 其余会落到这里）② 路径含空格未加引号 ③ 参数多打或打错。
	// 不拦的话它们被静默忽略，会造成"看起来成功、实际少传"（2026-09-14 实测）。
	if extra := fs.Args(); len(extra) > 0 {
		return nil, fmt.Errorf(
			"出现多余的参数（未被使用）：%s\n常见原因：\n"+
				"  ① 通配符被终端展开——`-u /x/abc/*` 会变成多个路径。上传目录不需要通配符，"+
				"直接写目录本身即可（目录内容会按原有层级传过去）\n"+
				"  ② 路径含空格但没加引号——请写成 \"路径 含 空格\"\n"+
				"  ③ 参数打多了或打错了",
			strings.Join(extra, " "))
	}

	// -t / -T / -n：转 int，非法留给 CheckArguments 报错
	a.Timeout, a.timeoutInvalid = toInt(a.timeoutRaw)
	a.ConnectTimeout, a.connectTimeoutInvalid = toInt(a.connectTimeoutRaw)
	a.Number, a.numberInvalid = toInt(a.numberRaw)

	// 未指定时的默认值（与旧版一致）
	switch {
	case a.Command != "" || a.Script != "":
		if a.timeoutRaw == "" {
			a.Timeout = cfg.Execution.TimeoutExecute
		}
	case a.Upload != "" || a.Download != "":
		if a.timeoutRaw == "" {
			a.Timeout = cfg.Execution.TimeoutTransfer
		}
	}
	if a.connectTimeoutRaw == "" {
		a.ConnectTimeout = cfg.Execution.TimeoutConnect
	}
	if a.Mode == "" {
		a.Mode = cfg.Execution.Mode
	}
	if a.numberRaw == "" {
		a.Number = 0
	}

	// 路径参数：中间不能含空格；再做字符串层规范化（顺序与旧版一致）
	for _, val := range []*string{&a.Script, &a.CsvFile, &a.Upload, &a.Path, &a.Download} {
		if *val == "" {
			continue
		}
		if strings.Contains(strings.TrimSpace(*val), " ") {
			return nil, fmt.Errorf("路径参数中间不能包含空格")
		}
		*val = normalizePath(*val)
	}

	a.Remark = defaultRemark(&a)
	return &a, nil
}

// normalizeKeyFlag 裸 -k 预处理：后随参数空缺或为另一选项时，改写为哨兵形式。
// 带路径的 `-k <path>`（空格或连写）保持原样，交给 pflag 原生消费。
// pflag 的 NoOptDefVal 恒优先于消费下一参数，会吞掉 `-k <path>` 空格形式，
// 故不能用（见 spec 依赖清单备注）。
func normalizeKeyFlag(raw []string) []string {
	out := make([]string, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == "-k" && (i+1 >= len(raw) || strings.HasPrefix(raw[i+1], "-")) {
			out = append(out, "-k="+keyModeSentinel)
			continue
		}
		out = append(out, raw[i])
	}
	return out
}

func toInt(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, true
	}
	return v, false
}

// normalizePath 字符串层面的路径规范化（对位旧 args_normalize_path）：
// 统一分隔符为 /、处理 . 与 ..、不增删结尾 /、Windows 盘符转大写 C:/ 形式。
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "\\", "/")
	endsSlash := strings.HasSuffix(p, "/")

	if len(p) >= 2 && p[1] == ':' && isDriveLetter(p[0]) {
		rest := strings.TrimLeft(p[2:], "/")
		if rest == "" {
			return strings.ToUpper(p[:1]) + ":/"
		}
		return strings.ToUpper(p[:1]) + ":/" + rest
	}

	cleaned := path.Clean(p)
	if endsSlash && cleaned != "/" {
		cleaned += "/"
	}
	return cleaned
}

func isDriveLetter(b byte) bool { return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') }

// defaultRemark 未指定 -r 时自动生成：命令取首词前 8 字符（特殊字符换下划线），
// 脚本 / 上传 / 下载取路径文件名。
func defaultRemark(a *Args) string {
	if a.Remark != "" {
		return a.Remark
	}
	switch {
	case a.Command != "":
		fields := strings.Fields(a.Command)
		if len(fields) == 0 {
			return ""
		}
		first := []rune(fields[0])
		if len(first) > 8 {
			first = first[:8]
		}
		return strings.NewReplacer(
			" ", "_", "/", "_", ":", "_", "*", "_", "?", "_",
			"\"", "_", "<", "_", ">", "_", "|", "_", "\\", "_",
		).Replace(string(first))
	case a.Script != "":
		return path.Base(a.Script)
	case a.Upload != "":
		return path.Base(a.Upload)
	case a.Download != "":
		return path.Base(a.Download)
	}
	return ""
}

// Usage 打印帮助（未提供任何参数或 -h 时）。默认值从配置插值（与旧版一致）；
// 首行下方显示入口定义的版本号。
func Usage(cfg *config.Config, version string) {
	name := filepath.Base(os.Args[0])
	entries := [][3]string{
		{"-c", "(命令模式)", "远程在多台服务器上执行一条命令"},
		{"-s", "(脚本模式)", "远程在多台服务器上执行一个本地脚本"},
		{"-u", "(上传模式)", "把本地文件或目录传到服务器"},
		{"-d", "(下载模式)", "从服务器下载文件或目录到本地"},
		{"-f", "", "节点清单：CSV 文件路径，或直接在命令行写一行节点信息 (-c/-s/-u/-d 时必须带)"},
		{"-p", "", "目标路径：上传到服务器的目录 / 从服务器下载到的本地目录 (-u/-d 时必须带)"},
		{"-m", fmt.Sprintf("[默认: %s]", cfg.Execution.Mode), "执行身份: direct=用登录用户身份, sudo=用 root 身份执行"},
		{"-t", fmt.Sprintf("[默认: 命令%ds/上传%ds]", cfg.Execution.TimeoutExecute, cfg.Execution.TimeoutTransfer), "单台执行或传输的超时时间 (秒)"},
		{"-T", fmt.Sprintf("[默认: %d]", cfg.Execution.TimeoutConnect), "连接每台服务器的超时时间 (秒)"},
		{"-n", "[默认: 同时跑全部节点]", "并发数：同时操作几台服务器 (不填则全部并行)"},
		{"-r", "", "给这次任务起个名字，会作为历史记录文件夹的后缀 (不填自动生成)"},
		{"--nobash", "", "命令模式专用: 不套一层 bash 环境，直接执行原始命令"},
		{"--disinteractive", "", "跳过所有确认提示直接执行 (批量跑脚本时常用)"},
		{"-k", "(密钥登录)", "不指定=纯密码; 仅 -k=用CSV/配置默认密钥; -k 路径=所有节点统一私钥"},
		{"--gen-key", "(密钥管理)", "生成随机主密钥并持久化到系统环境变量 SSHFLEET_KEY（凭据加密用）"},
		{"--convert-password", "(密钥管理)", "转换凭据文件（后面跟目标文件路径）：自动识别明文/base64/加密格式并按配置等级转换，支持升降级；加密/解密需已配置主密钥；路径支持相对 secret_dir"},
	}
	col2 := 0
	for _, e := range entries {
		if w := lipgloss.Width(e[1]); w > col2 {
			col2 = w
		}
	}
	var b strings.Builder
	b.WriteString("SSHFleet - 批量 SSH 运维工具（命令/脚本执行、文件上传下载）\n")
	b.WriteString(fmt.Sprintf("版本: v%s\n\n", version))
	b.WriteString("用法:\n")
	b.WriteString(fmt.Sprintf("  %s ( -c | -s | -u | -d ) ( -f ) ( -p ) [其他可选参数]   批量执行（四种模式四选一）\n", name))
	b.WriteString(fmt.Sprintf("  %s --gen-key | --convert-password 文件路径               工具选项（单独使用）\n\n", name))
	b.WriteString("选项:\n")
	for _, e := range entries {
		tag := e[1]
		if tag != "" {
			tag += strings.Repeat(" ", col2-lipgloss.Width(e[1]))
		} else {
			tag = strings.Repeat(" ", col2)
		}
		b.WriteString(fmt.Sprintf("  %-6s %s  %s\n", e[0], tag, e[2]))
	}
	b.WriteString("\n示例:\n")
	b.WriteString(fmt.Sprintf("  命令模式: %s -f nodes.csv -c \"ls -l\"\n", name))
	b.WriteString(fmt.Sprintf("  脚本模式: %s -f nodes.csv -s script.sh\n", name))
	b.WriteString(fmt.Sprintf("  上传模式: %s -f nodes.csv -u /local/path -p /remote/path/\n", name))
	b.WriteString(fmt.Sprintf("  下载模式: %s -f nodes.csv -d /remote/path -p /local/path\n", name))
	b.WriteString("\n上传并发说明:\n")
	b.WriteString("  上传模式下，工具根据配置文件中的文件大小阈值输出建议并发数\n")
	b.WriteString("  输入 y 使用建议值，输入 n 保留原值继续执行\n")
	fmt.Print(b.String())
}
