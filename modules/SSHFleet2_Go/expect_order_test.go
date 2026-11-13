package main

// 期望表：检查与提示的先后（议题 2026-11-02-1132，ADR-0052）。
//
// 站在塔尖的进程出入口上：起二进制、喂 argv、把 stdout 与 stderr 合起来读——顺序是屏幕的
// 性质，站内的测试看不见它。这是自定义区（文件名不带 `redline_` 前缀，不进红线名单），
// 断言的是「哪一句先说」这个值；红线区只判形状。
//
// 每行三样：输入、该先出现的那一格、其余各格（出现也不许比它更早）。哪一行不对只报哪一行。
// 表本身的底本在议题的 `spec.md`，同一行的号与那一份对得上。
//
// 与 Git Bash 无关：argv 由 exec 直接交给进程，不经过 shell，所以 `/opt/app/` 不会被改写成
// `C:/…`。环境里那个变量照旧设上——从别处（比如 bash）复跑同一份用例时它才是关键。

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// orderRow 是期望表的一行。
type orderRow struct {
	n          int      // 表里的行号（与 spec 那张表同号）
	layer      string   // 层号，报错时带上
	args       []string // 这次喂给二进制的 argv
	want       string   // 该先出现的那一格的特征串
	notEarlier []string // 其余各格：出现也不许比 want 更早
	after      string   // 要在 want 之后才出现（延后的那一句）
	stdin      string   // 要回答的输入（不写就是读到输入结束）
	awaiting   string   // 还没做：留给哪条工单；非空时只记现状，判不了红
	stage      func(t *testing.T, dir string)
}

// orderRows 是整张表。清空一行的 awaiting，就是那条工单把它接过去了。
var orderRows = []orderRow{
	// 第一层：词法
	{1, "1", []string{"--zzz"}, "不认识的选项", nil, "", "", "", nil},
	{2, "1", []string{"--gen-key", "--zzz"}, "不认识的选项", []string{"只能给一个"}, "", "", "", nil},
	{3, "1", []string{"-c"}, "缺少值", nil, "", "", "", nil},
	{4, "1", []string{"-c", "ls", "extra"}, "后面还有一段没法识别", nil, "", "", "", nil},
	{5, "1", []string{"-h", "--bogus"}, "批量 SSH 运维工具", []string{"不认识的选项"}, "", "", "", nil},
	{6, "1", []string{"--bogus", "-h"}, "不认识的选项", []string{"批量 SSH 运维工具"}, "", "", "", nil},
	{7, "1", []string{"--command=ls", "-h"}, "批量 SSH 运维工具", []string{"不认识的选项"}, "", "", "", nil},

	// 第二层：结构
	{8, "2", []string{"-f", "ok.csv"}, "你没有指定任何模式", nil, "", "", "", nil},
	{9, "2", []string{"-c", "ls", "-d", "/etc/passwd"}, "只能指定一个", nil, "", "", "", nil},
	{10, "2", []string{"--gen-key", "--key-status"}, "只能给一个", nil, "", "", "", nil},
	{11, "2", []string{"--gen-key", "-c", "ls"}, "不能和批量执行选项一起用", nil, "", "", "", nil},
	{12, "2", []string{"--gen-key", "-f", "ok.csv"}, "不能和批量执行选项一起用", nil, "", "", "", nil},
	{13, "1", []string{"--key-status", "extra"}, "出现多余的参数", []string{"只能给一个"}, "", "", "", nil},

	// 第三层：搭配、取值与必填
	{14, "3", []string{"-c", "ls", "-p", "/opt/app/", "-f", "ok.csv"}, "不能搭配 -c 或 -s",
		[]string{"文件读不到", "读取内容时发生错误"}, "", "", "02", nil},
	{15, "3", []string{"-c", " "}, "-c 参数不能为空", []string{"读取内容时发生错误"}, "", "", "02", nil},
	{16, "3", []string{"-u", "."}, "-u 参数必须搭配 -p", []string{"读取内容时发生错误"}, "", "", "02", nil},
	{17, "3", []string{"-u", ".", "-p", "rel"}, "必须是绝对路径", []string{"读取内容时发生错误"}, "", "", "02", nil},
	{18, "3", []string{"-u", ".", "-p", "/opt/app"}, "以 / 结尾", []string{"读取内容时发生错误"}, "", "", "02", nil},
	{19, "3", []string{"-d", "/etc/passwd"}, "-d 参数必须搭配 -p", []string{"读取内容时发生错误"}, "", "", "02", nil},
	{20, "3", []string{"-d", "rel", "-p", "./logs/"}, "必须是绝对路径", []string{"读取内容时发生错误"}, "", "", "02", nil},
	{21, "3", []string{"--sudo", "--no-sudo", "-c", "ls"}, "--sudo 与 --no-sudo 只能给一个",
		[]string{"读取内容时发生错误"}, "", "", "02", nil},
	{22, "3", []string{"--change-password", "pw", "-p", "/opt/app/"}, "不能和 -p 一起用",
		[]string{"读取内容时发生错误"}, "", "", "02", nil},
	{23, "3", []string{"--change-password", "pw", "-a", "y,x"}, "不能和 -a 一起用",
		[]string{"读取内容时发生错误"}, "", "", "02", nil},
	{24, "3", []string{"--change-password", "pw", "-k"}, "不能和 -k 一起用",
		[]string{"读取内容时发生错误"}, "", "", "02", nil},
	{25, "3", []string{"--change-password", " "}, "新密码不能为空",
		[]string{"读取内容时发生错误"}, "", "", "02", nil},
	{26, "3", []string{"-n", "abc", "-c", "ls"}, "-n 参数格式错误", nil, "", "", "", nil},
	{27, "3", []string{"-t", "abc", "-c", "ls", "-f", "/nope.csv"}, "-t 参数格式错误",
		[]string{"文件读不到"}, "", "", "", nil},
	{28, "3", []string{"-T", "abc", "-c", "ls", "-f", "/nope.csv"}, "-T 参数格式错误",
		[]string{"文件读不到"}, "", "", "", nil},
	{29, "3", []string{"-c", "ls"}, "没有给节点清单", []string{"读取内容时发生错误"}, "", "", "03", nil},
	{30, "3", []string{"-s", "./x.xyz", "-f", "ok.csv"}, "继续用 bash",
		[]string{"发现禁止命令"}, "", "n", "03", nil},
	{31, "3", []string{"-c", "rm -rf /", "-p", "/opt/app/", "-f", "ok.csv"}, "不能搭配 -c 或 -s",
		[]string{"发现禁止命令"}, "", "", "02", nil},

	// 第四层：判决
	{32, "4", []string{"-c", "rm -rf /", "-f", "ok.csv"}, "发现禁止命令", nil, "", "", "", nil},
	{33, "4", []string{"-c", "rm -rf /", "-f", "/nope.csv"}, "发现禁止命令",
		[]string{"文件读不到"}, "", "", "02", nil},
	{34, "4", []string{"-c", "rm -rf /", "-f", "bad.csv"}, "发现禁止命令",
		[]string{"双引号"}, "", "", "02", nil},
	{35, "4", []string{"-c", "RM -RF /etc", "-f", "ok.csv"}, "发现禁止命令", nil, "", "", "", nil},
	{36, "4", []string{"-c", "rm -rf //", "-f", "ok.csv"}, "发现禁止命令", nil, "", "", "", nil},
	{37, "4", []string{"-c", "rm -rf /opt", "-f", "ok.csv"}, "发现危险命令",
		[]string{"发现禁止命令"}, "", "", "", nil},
	{38, "4", []string{"-c", "rm -rf /opt", "-f", "/nope.csv"}, "发现危险命令",
		[]string{"文件读不到"}, "文件读不到", "y", "02", nil},
	{39, "4", []string{"-c", "ls", "-f", "ok.csv"}, "读取危险命令检测规则", nil, "", "", "05", dropDangerRules},

	// 第五层：材料可达
	{40, "5", []string{"-s", "/nope.sh", "-f", "ok.csv"}, "脚本文件读不到", nil, "", "", "", nil},
	{41, "5", []string{"-u", "/nope", "-p", "/opt/app/", "-f", "ok.csv"}, "上传文件或目录读不到", nil, "", "", "", nil},
	{42, "5", []string{"-u", "/nope", "-p", "/opt/app/", "-f", "/nope.csv"}, "上传文件或目录读不到",
		[]string{"-f 参数指定的文件读不到"}, "", "", "02", nil},
	{43, "5", []string{"-d", "/x", "-p", "./nope", "-f", "ok.csv"}, "本地目录读不到", nil, "", "", "", nil},
	{44, "5", []string{"-c", "ls", "-f", "/nope.csv"}, "-f 参数指定的文件读不到", nil, "", "", "", nil},
	{45, "5", []string{"-c", "ls", "-f", "empty.csv"}, "CSV 文件为空", nil, "", "", "", nil},
	{46, "5", []string{"-c", "ls", "-f", "bin.csv"}, "清单是二进制文件", nil, "", "", "", nil},

	// 第六层：材料内容
	{47, "6", []string{"-c", "ls", "-f", "bad.csv"}, "双引号要成对", nil, "", "", "", nil},
	{48, "6", []string{"-c", "ls", "-f", "bad-ip.csv"}, "IP格式不正确", nil, "", "", "", nil},
	{49, "6", []string{"-c", "ls", "-f", "bad-cred.csv"}, "凭据", nil, "", "", "05", nil},

	// 第七层：人点头
	{50, "7", []string{"--sudo", "--no-shell", "-c", "ls", "-f", "ok.csv"}, "--no-shell 是原样下发",
		nil, "", "n", "", nil},
	{51, "7", []string{"-a", "确认,y", "-c", "ls", "-f", "ok.csv"}, "汉字",
		nil, "", "n", "05", englishOn},
	{52, "7", []string{"-c", "ls", "-f", "ok.csv"}, "执行选项/参数确认", nil, "", "n", "", nil},

	// 提示的位置（跨层，单列）
	{53, "3", []string{"-c", "ls", "-p", "/opt/app/", "-f", "hdr.csv"}, "不能搭配 -c 或 -s",
		[]string{"已移除表头"}, "", "", "04", nil},
	{54, "7", []string{"-c", "ls", "-f", "hdr.csv"}, "已移除表头",
		[]string{"执行选项/参数确认"}, "", "n", "04", nil},
	{55, "7", []string{"-c", "ls", "-f", "hdr.csv", "--yes"}, "已移除表头",
		nil, "跳过", "", "04", nil},
}

// orderSandbox 是二进制与临时目录的落点：`build/` 下当天的沙盒，不出工作区。
func orderSandbox(t *testing.T) string {
	t.Helper()
	module, err := os.Getwd()
	if err != nil {
		t.Fatalf("取当前目录失败：%v", err)
	}
	dir := filepath.Join(module, "..", "..", "build", "期望表_"+time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("建沙盒失败：%v", err)
	}
	return filepath.Clean(dir)
}

var (
	orderBinOnce sync.Once
	orderBinPath string
	orderBinErr  error
)

// orderBinary 编一次二进制，整张表共用。
func orderBinary(t *testing.T) string {
	t.Helper()
	orderBinOnce.Do(func() {
		module, err := os.Getwd()
		if err != nil {
			orderBinErr = err
			return
		}
		name := "sshfleet2_order"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		orderBinPath = filepath.Join(orderSandbox(t), name)
		build := exec.Command("go", "build", "-o", orderBinPath, ".")
		build.Dir = module
		if out, err := build.CombinedOutput(); err != nil {
			orderBinErr = fmt.Errorf("编二进制失败：%v\n%s", err, out)
		}
	})
	if orderBinErr != nil {
		t.Fatalf("%v", orderBinErr)
	}
	return orderBinPath
}

// stageOrderDir 给一行造一个自己的目录：主配置 ＋ 三份判据表 ＋ 那份夹具。
//
// 判据表照抄工程自带的那三份（它是数据，不是被测代码）；主配置与夹具由这里写死。
func stageOrderDir(t *testing.T, module string, r orderRow) string {
	t.Helper()
	dir := t.TempDir()

	cfg := filepath.Join(dir, "config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatalf("建配置目录失败：%v", err)
	}
	for _, name := range []string{"keywords_dangerous.conf", "keywords_error.conf", "keywords_passwd.conf"} {
		data, err := os.ReadFile(filepath.Join(module, "config", name))
		if err != nil {
			t.Fatalf("读判据表 %s 失败：%v", name, err)
		}
		if err := os.WriteFile(filepath.Join(cfg, name), data, 0o644); err != nil {
			t.Fatalf("写判据表 %s 失败：%v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(cfg, "SSHFleet.conf"), []byte(orderConfig), 0o644); err != nil {
		t.Fatalf("写主配置失败：%v", err)
	}

	// 夹具：行里点名的那几份。「./nope」故意不建——它就是要不存在。
	for name, body := range map[string]string{
		"ok.csv":       "192.0.2.10,10022,root,pw\n",
		"hdr.csv":      "ip,port,user,password\n192.0.2.10,10022,root,pw\n",
		"bad.csv":      "192.0.2.10,10022,\"root,pw\n",
		"empty.csv":    "",
		"bin.csv":      "192.0.2.10,10022,root,pw\x00bin\n",
		"bad-ip.csv":   "192.0.2.10,10022,root,pw\n999.1.1.1,10022,root,pw\n",
		"bad-cred.csv": "192.0.2.10,10022,root,/nope/cipher\n",
		// x.xyz 的正文要命中危险规则：回退那一问与判决的先后，只有正文危险时才看得出来。
		"x.xyz": "#!/bin/bash\nrm -rf /\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("写夹具 %s 失败：%v", name, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o755); err != nil {
		t.Fatalf("建 ./logs/ 失败：%v", err)
	}

	if r.stage != nil {
		r.stage(t, dir)
	}
	return dir
}

// orderConfig 是这一族用例的主配置：加密关（主密钥不掺和）、English 关、口令明文。
const orderConfig = `[account]
port = 10022
user = "root"
password = "pw"

[credential]
encrypt = false
key = ""
key_password = ""
secret_dir = ""

[execution]
sudo = true
timeout_connect = 1
timeout_execute = 5
timeout_transfer = 5

[enable]
output_to_xlsx = false
results_to_xlsx = false
show_category_tips = true

[interactive]
regex = false
case_sensitive = false

[env]
PATH = "/usr/bin"

[locale]
english = false

[interpreter]
command = "bash"

[interpreter.script]
".sh" = "bash"

[upload]
small_file = 1048576
large_file = 10485760
medium_parallel = 4
`

// dropDangerRules 把危险规则表移走：判据不全时该报「工具没能力判断」，不是「你参数错了」。
func dropDangerRules(t *testing.T, dir string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, "config", "keywords_dangerous.conf")); err != nil {
		t.Fatalf("移走危险规则表失败：%v", err)
	}
}

// englishOn 把提示语切成英文：english 开着而代填值里有汉字，一定失配。
func englishOn(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "config", "SSHFleet.conf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读主配置失败：%v", err)
	}
	turned := strings.Replace(string(data), "english = false", "english = true", 1)
	if turned == string(data) {
		t.Fatalf("主配置里没有 english = false 这一行")
	}
	if err := os.WriteFile(path, []byte(turned), 0o644); err != nil {
		t.Fatalf("写主配置失败：%v", err)
	}
}

// TestOrderExpectedFirstMessage 是整张期望表：一行一组输入，判「哪一句先说」。
func TestOrderExpectedFirstMessage(t *testing.T) {
	module, err := os.Getwd()
	if err != nil {
		t.Fatalf("取当前目录失败：%v", err)
	}
	bin := orderBinary(t)

	for _, r := range orderRows {
		r := r
		t.Run(fmt.Sprintf("%02d", r.n), func(t *testing.T) {
			dir := stageOrderDir(t, module, r)

			cmd := exec.Command(bin, r.args...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "MSYS2_ARG_CONV_EXCL=*")
			if r.stdin != "" {
				cmd.Stdin = strings.NewReader(r.stdin + "\n")
			}
			var buf strings.Builder
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			runErr := cmd.Run()
			if runErr != nil {
				if _, ok := runErr.(*exec.ExitError); !ok {
					t.Fatalf("起二进制失败：%v", runErr)
				}
			}
			out := buf.String()

			if r.awaiting != "" {
				// 还没做的那几行：只记现状，不判红。清空 awaiting 才算接手。
				t.Logf("第 %d 行（第 %s 层）留给 %s 号单，现状：%s", r.n, r.layer, r.awaiting, orderVerdict(r, out))
				return
			}
			if why := orderVerdict(r, out); why != "" {
				t.Fatalf("第 %d 行（第 %s 层）：%s\n输入：%v\n输出：\n%s", r.n, r.layer, why, r.args, out)
			}
		})
	}
}

// orderVerdict 判一行，返回空串表示过。
func orderVerdict(r orderRow, out string) string {
	at := strings.Index(out, r.want)
	if at < 0 {
		return fmt.Sprintf("该先报的那一格没出现：%q", r.want)
	}
	for _, other := range r.notEarlier {
		if i := strings.Index(out, other); i >= 0 && i < at {
			return fmt.Sprintf("%q 抢在 %q 之前出现", other, r.want)
		}
	}
	if r.after != "" && !strings.Contains(out[at:], r.after) {
		return fmt.Sprintf("%q 该在 %q 之后出现，没看到", r.after, r.want)
	}
	return ""
}
