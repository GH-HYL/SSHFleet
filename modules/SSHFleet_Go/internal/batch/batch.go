// Package batch 承载主干第 8 步：并发执行 SSH / SFTP + 进度聚合。
//
// 进度聚合器住这里（跨调用保存状态、与 worker pool 同一生命周期，spec D2）；
// 渲染由 main 把 internal/output 的函数作为参数注入——依赖注入，不是回调控制生命周期。
package batch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sshfleet/internal/cli"
	"sshfleet/internal/config"
	"sshfleet/internal/log"
	"sshfleet/internal/nodelist"
	"sshfleet/internal/ssh"
	"sshfleet/internal/verdict"
)

// Results 本轮执行的节点结果集合（按 Seq 保序）。
type Results struct{ Items []ssh.Result }

func (r *Results) Len() int { return len(r.Items) }

// task 单个节点的执行任务（五种模式共用：命令 / 脚本 / 上传 / 下载 / 改密）。
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

	// interactive 非空即走交互分支（给了 -a）：正文经命令行承载、会话 stdin 让给代填
	interactive *ssh.InteractiveInput

	// passwd 非空即走改密分支（给了 --change-password）：会话里按提示词表喂入
	passwd *ssh.PasswdInput
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
//
// prompts 是改密的提示词表（用户可调的规则文件，由 main 加载后传下来）。
// kw 是错误分类判据表，两处在用：代填拿「密码过期」当**中止词**、改密拿它当**入场信号**
// （构建任务时从表里抽出）；每个节点的结果判定（verdict.Judge）也在 worker 协程里
// 用它做完再交给通道（D9）——main 不必再手工抽关键词传下来。
//
// upload 是上传模式下**上游已采集好的**清单（其余模式为 nil）：采集提到确认之前做、
// 这里只用结果，不再重复遍历——确认屏与最终传输同源，采集期的错误也在确认之前就报出。
func Run(ctx context.Context, a *cli.Args, cfg *config.Config, nodes *nodelist.Nodes, logger *log.Logger, prompts *ssh.PasswdPrompts, kw *verdict.Keywords, upload *CollectResult, hooks Hooks) (*Results, error) {
	tasks, notices, err := buildTasks(a, nodes, prompts, kw, upload)
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
	// 执行参数：两个超时与执行身份。`-t` / `-T` 未显式给值时由选项/参数解析按模式填配置默认值，
	// 事后光看命令行原文看不出实际生效的是多少；执行身份由 `--sudo` / `--no-sudo` 与配置
	// 共同决定，终值同样不在命令行原文里。记在这里（单线程区、协程未起），与「开始执行任务」
	// 一起构成这一轮的现场。
	logger.Info(fmt.Sprintf("执行参数：执行超时 %d 秒，连接超时 %d 秒，执行身份 %s",
		a.Timeout, a.ConnectTimeout, identityLabel(a.Sudo)))

	logger.Info(fmt.Sprintf("开始执行任务：节点 %d 个，并发 %d，模式 %s", len(tasks), concurrency, a.ModeName().Info().LogName))

	// 代填表逐条明细（内容 → 触发词）：-a 现场排查的正据——「这次配了哪几条、触发词各是什么」
	// 一眼可见，不必回头翻命令行。非 -a 无表，一行不写。
	for _, line := range answerTable(a) {
		logger.Success(line)
	}

	// 交代命令被包成了什么（旧 Python builder.py 的「完整命令拼接完成」对应物）：
	// 命令走 stdin 通道后，命令行里只剩固定形态的 sh -c，事后看日志查不出
	// 「原始命令是什么、被包成了哪一行」，脚本模式连解释器与身份都无从确认。
	for _, line := range commandDescription(a, tasks) {
		logger.Success(line)
	}

	agg := NewAggregator(hooks.OnProgress)
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

	mode := a.ModeName()
	work := func(ctx context.Context, t *task) ssh.Result {
		client := ssh.NewClient(newConfig(t.node))
		onProgress := func(p ssh.Progress) { agg.OnProgress(p) }

		var res *ssh.Result
		switch {
		case a.Upload != "":
			res = client.UploadFiles(ctx, t.files, t.skipped, t.remote, t.useSudo, t.seq, onProgress)
		case a.Download != "":
			res = client.DownloadFiles(ctx, t.remote, t.local, t.useSudo, t.seq, onProgress)
		case t.passwd != nil:
			res = client.RunPasswdChange(ctx, *t.passwd, t.seq)
		case t.interactive != nil:
			res = client.RunInteractive(ctx, *t.interactive, t.command, t.seq)
		default:
			res = client.RunCommand(ctx, t.command, t.stdin, t.seq)
		}
		// 判定在 worker 协程里做完再交给通道（D9）：下游（进度聚合 / 结果流水 /
		// 统计 / 呈现）拿到的都是带结论的完整结果。
		verdict.Judge(res, mode, kw)
		return *res
	}

	// 「完成」只有一个收口点：真正进了结果通道的结果才算数，聚合器的计数与
	// 统计读到的切片由此同源。此前在 worker 里先 agg.OnResult、再往通道送，
	// 而取消时通道的 select 会随机丢弃，进度数因此比统计数多（2026-10-07 审计候选二）。
	onResult := func(r ssh.Result) {
		agg.OnResult(r)
		if hooks.OnResult != nil {
			hooks.OnResult(r)
		}
	}
	results := runPool(ctx, concurrency, tasks, work, onResult)
	sortBySeq(results)
	return &Results{Items: results}, nil
}

// buildTasks 按模式构建任务；需要读取本地资源的错误在此一次性暴露（尚未建连）。
// 上传模式的清单由调用方（主干）在**确认之前**采集好传进来（upload），这里不再自行遍历。
// 第二个返回值是采集期提示（如上传源中被过滤的软链接数量与名称）。
func buildTasks(a *cli.Args, nodes *nodelist.Nodes, prompts *ssh.PasswdPrompts, kw *verdict.Keywords, upload *CollectResult) ([]*task, []string, error) {
	tasks := make([]*task, 0, nodes.Len())
	var notices []string
	// 「密码过期」分类的关键词，两条路共用：代填拿它当中止词，改密拿它当入场信号。
	expiredKeywords := kw.KeywordsOf("密码过期")

	switch {
	case a.ChangePassword != "":
		// 改密：全场一份入场值——新密码全局同一个，提示词表与匹配口径也全场同一套。
		// 当前密码不在这里传：它就是每台机器自己的登录密码，客户端配置里已经有了。
		// 过期信号在这里是入场信号，不是中止词（改密会话不带中止词）。
		in := ssh.PasswdInput{
			NewPassword:   a.NewPassword,
			Prompts:       prompts,
			EnterKeywords: expiredKeywords,
			Match:         a.Match,
		}
		for i, node := range nodes.Items {
			tasks = append(tasks, &task{seq: i, node: node, passwd: &in})
		}
	case a.Upload != "":
		// 清单在确认之前就采集好了（main 传进来的唯一一份），这里只用结果：重复遍历会让
		// 「确认屏看到的」与「实际要传的」各算各的，采集期的错误也会拖到按下确认之后才报。
		if upload == nil {
			return nil, nil, fmt.Errorf("上传清单缺失\n原因：主干未在上传模式下采集清单\n提示：这是内部错误，请把本次命令反馈给作者")
		}
		// 软链接等被过滤的条目不阻断执行，但要让用户知道（-u 走终端提示）
		if len(upload.Skipped) > 0 {
			notices = append(notices, fmt.Sprintf("[提示] 上传源中有 %d 个软链接/快捷方式被过滤（不上传）：%s",
				len(upload.Skipped), Summarize(upload.Skipped)))
		}
		for i, node := range nodes.Items {
			tasks = append(tasks, &task{seq: i, node: node, files: upload.Files, skipped: upload.Skipped, remote: a.Path, useSudo: a.Sudo})
		}
	case a.Download != "":
		// 本地落地目录转绝对路径（对位旧 Python builder）
		local, err := filepath.Abs(a.Path)
		if err != nil {
			return nil, nil, err
		}
		for i, node := range nodes.Items {
			tasks = append(tasks, &task{seq: i, node: node, remote: a.Download, local: local, useSudo: a.Sudo})
		}
	default: // 命令 / 脚本
		var material cli.ScriptMaterial
		if a.Script != "" {
			data, err := os.ReadFile(a.Script)
			if err != nil {
				return nil, nil, fmt.Errorf("读取脚本文件失败：%s\n原因：%v", a.Script, err)
			}
			material = cli.ScriptMaterialOf(a, data)
		}
		body := material.Body
		// 解释器在选项/参数解析阶段就算好了（命令模式 = 命令解释器；脚本模式 = 后缀映射或回退），
		// 这里只取用。空值说明解析链路有漏——拼接前拦下，不让一条没有解释器的命令发出去。
		interpreter := a.Interpreter
		if interpreter == "" {
			return nil, nil, fmt.Errorf("没有为这次执行解析出解释器\n提示：检查配置文件里的 [interpreter] 段")
		}
		name := scriptName(a)
		if len(a.Answers) > 0 {
			// 交互分支：正文改经命令行承载，会话 stdin 整条让给代填（ADR-0010）。
			// 下发行由选项/参数合规阶段拼好存进 Args.Downlink（那里要拿它做长度检查），
			// 这里直接取用——量的串、日志里那条、实际发出去那条是同一个值，不再重拼。
			if a.Downlink == "" {
				return nil, nil, fmt.Errorf("没有为这次交互执行拼出下发行\n" +
					"原因：选项/参数合规检查阶段未产出 Args.Downlink\n" +
					"提示：这是内部错误，请把本次命令反馈给作者")
			}
			// 执行期入场值只剩三样：代填表、匹配口径、中止词。
			in := ssh.InteractiveInput{
				Answers:       a.Answers,
				Match:         a.Match,
				AbortKeywords: expiredKeywords,
			}
			for i, node := range nodes.Items {
				tasks = append(tasks, &task{seq: i, node: node, command: a.Downlink, interactive: &in})
			}
			break
		}
		command, stdin := ssh.BuildCommand(a.Command, body, name, interpreter, a.EnvPrefix, a.NoShell, a.Sudo)
		for i, node := range nodes.Items {
			tasks = append(tasks, &task{seq: i, node: node, command: command, stdin: stdin})
		}
	}
	return tasks, notices, nil
}

// commandDescription 命令/脚本模式下「交给 SSH 执行的是什么」的日志行。
// 上传 / 下载模式没有命令可交代，返回 nil。
func commandDescription(a *cli.Args, tasks []*task) []string {
	if len(tasks) == 0 || (a.Command == "" && a.Script == "") {
		return nil
	}

	// 交互分支的下发行与 stdin 用途都与普通路径不同（正文在命令行里、stdin 让给代填），
	// 说明也换一份——日志的用处正是事后查得出这次到底发了什么。
	if t := tasks[0]; t.interactive != nil {
		return indentLines(ssh.DescribeInteractive(ssh.DescribeInteractiveInput{
			Command:    a.Command,
			ScriptPath: a.Script,
			Answers:    len(t.interactive.Answers),
			Downlink:   t.command,
		}))
	}

	interpreter := a.Interpreter

	// 脚本模式下正文非空即判定为脚本模式（DescribeCommand 的唯一依据）
	body := tasks[0].stdin
	if a.Command != "" {
		body = ""
	}

	return indentLines(ssh.DescribeCommand(ssh.DescribeInput{
		Command:     a.Command,
		ScriptPath:  a.Script,
		ScriptBody:  body,
		ScriptName:  scriptName(a),
		Interpreter: interpreter,
		EnvPrefix:   a.EnvPrefix,
		NoShell:     a.NoShell,
		AsRoot:      a.Sudo,
	}))
}

// scriptName 脚本文件名（basename）。下发行要把它交回去当 $0——这类脚本常从自己的
// 名字里取信息（目标 IP、站点类型），不补回去它们就取到空值，后面拿空值做数值比较
// 会直接报错。为什么是 basename 而不是原路径：脚本并没有落到目标机上，给一个本机
// 路径反而会让 dirname 那类写法指向不存在的地方。详见 ssh 侧 scriptNameArg 的说明。
func scriptName(a *cli.Args) string {
	if a.Script == "" {
		return ""
	}
	return filepath.Base(a.Script)
}

// identityLabel 执行身份的中文名（与参数屏「执行身份」行同口径）。
func identityLabel(sudo bool) string {
	if sudo {
		return "root"
	}
	return "登录用户"
}

// answerTable -a 代填表的逐条明细（内容 → 触发词）；非 -a 没有表，返回 nil。
// Describe 是代填的既有呈现形态（报告与参数屏共用），这里不再另拼一份写法。
func answerTable(a *cli.Args) []string {
	if len(a.Answers) == 0 {
		return nil
	}
	lines := make([]string, 0, len(a.Answers)+1)
	lines = append(lines, fmt.Sprintf("代填表（%d 条）：", len(a.Answers)))
	for i, ans := range a.Answers {
		lines = append(lines, fmt.Sprintf("  第 %d 条：%s", i+1, ans.Describe()))
	}
	return lines
}

// indentLines 日志正文逐行缩进两格（说明块在日志里自成一段）。
func indentLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := make([]string, 0, 4)
	for _, ln := range strings.Split(text, "\n") {
		lines = append(lines, "  "+ln)
	}
	return lines
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
