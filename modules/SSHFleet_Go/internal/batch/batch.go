// Package batch 承载主干第 8 步：并发执行 SSH / SFTP + 进度聚合。
//
// 进度聚合器住这里（跨调用保存状态、与 worker pool 同一生命周期，spec D2）；
// 渲染由 main 把 internal/output 的函数作为参数注入——依赖注入，不是回调控制生命周期。
package batch

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"sshfleet/internal/cli"
	"sshfleet/internal/config"
	"sshfleet/internal/log"
	"sshfleet/internal/nodelist"
	"sshfleet/internal/ssh"
)

// Results 本轮执行的节点结果集合（按 Seq 保序）。
type Results struct{ Items []ssh.Result }

func (r *Results) Len() int { return len(r.Items) }

// task 单个节点的执行任务（四种模式共用：命令 / 脚本 / 上传 / 下载）。
type task struct {
	seq     int
	node    nodelist.NodeInfo
	command string
	stdin   string
	files   []ssh.LocalFile
	skipped []string // 上传：采集阶段被过滤的链接（相对路径），随结果明细留痕
	remote  string
	local   string
	useSudo bool
}

// RenderFunc 进度渲染函数（由 main 从 internal/output 注入）。
type RenderFunc func(Snapshot)

// Hooks 执行期回调注入：采集提示 + 进度渲染 + 结果流水（写 output.txt / 执行日志 / 终端明细）。
type Hooks struct {
	// OnNotice 采集期提示（目前仅「上传源中被过滤的软链接」，用户 2026-09-14 裁定）。
	// 在进度界面之前回调，由 main 决定去向（终端）——batch 自己不打印。
	OnNotice   func(string)
	OnProgress RenderFunc
	OnResult   func(ssh.Result)
}

// Run 主干第 8 步入口：构建任务 → 并发执行 → 聚合进度 → 返回结果。
// logger 是执行期日志（此时工具日志已轮转至此），运行期事件写入这里而非工具日志。
func Run(ctx context.Context, a *cli.Args, cfg *config.Config, nodes *nodelist.Nodes, logger *log.Logger, hooks Hooks) (*Results, error) {
	tasks, notices, err := buildTasks(a, nodes)
	if err != nil {
		return nil, err
	}
	// 采集期提示先于任何进度事件下发，保证不被进度条重绘覆盖
	for _, msg := range notices {
		if hooks.OnNotice != nil {
			hooks.OnNotice(msg)
		}
	}

	concurrency := a.Number
	if concurrency <= 0 || concurrency > len(tasks) {
		concurrency = len(tasks)
	}
	logger.Info(fmt.Sprintf("开始执行任务：节点 %d 个，并发 %d，模式 %s", len(tasks), concurrency, execModeName(a)))

	// 交代命令被包成了什么（旧 Python builder.py 的「完整命令拼接完成」对应物）：
	// 命令走 stdin 通道后，命令行里只剩固定形态的 bash -lc，事后看日志查不出
	// 「原始命令是什么、被包成了哪一行」，脚本模式连解释器与身份都无从确认。
	for _, line := range commandDescription(a, tasks) {
		logger.Success(line)
	}

	agg := NewAggregator(len(tasks), hooks.OnProgress)
	// 先渲染一次 0% 的初始界面：命令模式没有字节级进度回调，首个进度事件要等
	// 第一个节点完成才来，此前屏幕上没有任何「执行中」的反馈（用户 2026-09-15 裁定）。
	// 采集期提示已在上面下发完毕，此刻建界面不会把它顶掉。
	agg.emit(agg.Snapshot(), true)
	newConfig := func(node nodelist.NodeInfo) *ssh.Config {
		return &ssh.Config{
			IP:             node.IP,
			Port:           node.Port,
			User:           node.User,
			Password:       node.Password,
			KeyContent:     node.KeyContent,
			KeyPassphrase:  node.KeyPassphrase,
			ConnectTimeout: seconds(a.ConnectTimeout),
			ExecTimeout:    seconds(a.Timeout),
		}
	}

	work := func(ctx context.Context, t *task) ssh.Result {
		client := ssh.NewClient(newConfig(t.node))
		onProgress := func(p ssh.Progress) { agg.OnProgress(p) }

		var res *ssh.Result
		switch {
		case a.Upload != "":
			res = client.UploadFiles(ctx, t.files, t.skipped, t.remote, t.useSudo, t.seq, onProgress)
		case a.Download != "":
			res = client.DownloadFiles(ctx, t.remote, t.local, t.useSudo, t.seq, onProgress)
		default:
			res = client.RunCommand(ctx, t.command, t.stdin, t.seq)
		}
		agg.OnResult(*res)
		return *res
	}

	results := runPool(ctx, concurrency, tasks, work, hooks.OnResult)
	sortBySeq(results)
	return &Results{Items: results}, nil
}

// buildTasks 按模式构建任务；需要读取本地资源的错误在此一次性暴露（尚未建连）。
// 第二个返回值是采集期提示（如上传源中被过滤的软链接数量与名称）。
func buildTasks(a *cli.Args, nodes *nodelist.Nodes) ([]*task, []string, error) {
	tasks := make([]*task, 0, nodes.Len())
	var notices []string

	switch {
	case a.Upload != "":
		// 上传源转绝对路径（对位旧 Python builder 的 os.path.abspath）
		src, err := filepath.Abs(a.Upload)
		if err != nil {
			return nil, nil, err
		}
		collected, err := CollectLocalFiles(src)
		if err != nil {
			return nil, nil, err
		}
		// 软链接等被过滤的条目不阻断执行，但要让用户知道（-u 走终端提示）
		if len(collected.Skipped) > 0 {
			notices = append(notices, fmt.Sprintf("提示：上传源中有 %d 个软链接/快捷方式被过滤（不上传）：%s",
				len(collected.Skipped), Summarize(collected.Skipped)))
		}
		for i, node := range nodes.Items {
			tasks = append(tasks, &task{seq: i, node: node, files: collected.Files, skipped: collected.Skipped, remote: a.Path, useSudo: a.Mode == "sudo"})
		}
	case a.Download != "":
		// 本地落地目录转绝对路径（对位旧 Python builder）
		local, err := filepath.Abs(a.Path)
		if err != nil {
			return nil, nil, err
		}
		for i, node := range nodes.Items {
			tasks = append(tasks, &task{seq: i, node: node, remote: a.Download, local: local, useSudo: a.Mode == "sudo"})
		}
	default: // 命令 / 脚本
		var body, interpreter string
		if a.Script != "" {
			data, err := os.ReadFile(a.Script)
			if err != nil {
				return nil, nil, fmt.Errorf("读取脚本文件失败：%s\n原因：%v", a.Script, err)
			}
			body = strings.TrimSpace(string(data))
			interpreter = "bash"
			if path.Ext(a.Script) == ".py" {
				interpreter = "python3"
			}
		}
		command, stdin := ssh.BuildCommand(a.Command, body, interpreter, a.NoBash, a.Mode == "sudo")
		for i, node := range nodes.Items {
			tasks = append(tasks, &task{seq: i, node: node, command: command, stdin: stdin})
		}
	}
	return tasks, notices, nil
}

// execModeName 模式名的中文文案（日志用）。模式本身由 cli.Args.ModeName 判定，
// 这里只做「模式名 → 文案」的映射，不再重判 Args 的字段。
func execModeName(a *cli.Args) string {
	switch a.ModeName() {
	case "command":
		return "命令"
	case "script":
		return "脚本"
	case "upload":
		return "上传"
	case "download":
		return "下载"
	}
	return "未知"
}

// commandDescription 命令/脚本模式下「命令被包成了什么」的日志行。
// 上传 / 下载模式没有命令可交代，返回 nil。
//
// 命令原文取自首个任务（同一轮下发的命令对所有节点相同，节点间不做区分）。
func commandDescription(a *cli.Args, tasks []*task) []string {
	if len(tasks) == 0 || (a.Command == "" && a.Script == "") {
		return nil
	}

	interpreter := ""
	if a.Script != "" {
		interpreter = "bash"
		if path.Ext(a.Script) == ".py" {
			interpreter = "python3"
		}
	}

	// 脚本正文从首个任务取（buildTasks 已读盘并按 CRLF 清理），不再读第二遍
	body := tasks[0].stdin
	if a.Command != "" {
		body = ""
	}

	text := ssh.DescribeCommand(ssh.DescribeInput{
		Command:     a.Command,
		ScriptPath:  a.Script,
		ScriptBody:  body,
		Interpreter: interpreter,
		Identity:    identityText(a.Mode),
		NoBash:      a.NoBash,
		AsRoot:      a.Mode == "sudo",
	})
	if text == "" {
		return nil
	}

	lines := []string{"完整命令拼接完成"}
	for _, ln := range strings.Split(text, "\n") {
		lines = append(lines, "  "+ln)
	}
	return lines
}

// identityText 执行身份的中文说法（日志用）。-m 的取值合法性已由 CheckArguments 保证，
// 这里只翻文案。
func identityText(mode string) string {
	if mode == "sudo" {
		return "root（sudo 提权）"
	}
	return "登录用户（direct）"
}

func seconds(v int) time.Duration { return time.Duration(v) * time.Second }

// sortBySeq 结果按 Seq 保序（worker 完成顺序不定）。
func sortBySeq(results []ssh.Result) {
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j-1].Seq > results[j].Seq; j-- {
			results[j-1], results[j] = results[j], results[j-1]
		}
	}
}
