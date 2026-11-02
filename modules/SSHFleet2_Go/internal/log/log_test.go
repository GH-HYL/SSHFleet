package log

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 一行日志的形态：时间戳  -  [级别]  -  正文。分隔符两侧各两个空格，与老版逐字一致——
// 老版那几份日志是要发给别人看的，多一个空格就与历史对不上。
var lineShape = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}  -  \[INFO   \]  -  正文$`)

func TestInitExecWritesLineInOldShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SSHFleetExec.log")

	lg, err := InitExec(path)
	if err != nil {
		t.Fatalf("开执行期日志失败：%v", err)
	}
	lg.Info("正文")
	if err := lg.Close(); err != nil {
		t.Fatalf("关不上：%v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读不到执行期日志：%v", err)
	}
	line := strings.TrimRight(string(body), "\n")
	if !lineShape.MatchString(line) {
		t.Fatalf("这一行的形态不对：%q", line)
	}
}

// 同一个路径再开一次是**追加**，不是重写：老版一次运行里日志会轮转过去，中途断了要留着。
func TestInitExecAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SSHFleetExec.log")

	for _, text := range []string{"第一行", "第二行"} {
		lg, err := InitExec(path)
		if err != nil {
			t.Fatalf("开执行期日志失败：%v", err)
		}
		lg.Info(text)
		if err := lg.Close(); err != nil {
			t.Fatalf("关不上：%v", err)
		}
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读不到执行期日志：%v", err)
	}
	if !strings.Contains(string(body), "第一行") || !strings.Contains(string(body), "第二行") {
		t.Fatalf("后一次把前一次盖掉了：\n%s", body)
	}
}

// 目录不存在时**不替人补目录**：目录归归档包建，补了就把「上游没给对」这件事藏起来了。
func TestInitExecDoesNotCreateDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "没有这个目录", "SSHFleetExec.log")
	if _, err := InitExec(path); err == nil {
		t.Fatal("目录不存在时应当报错，而不是默默建一个")
	}
}

// 工具日志那条路仍然自己建目录（它的历史根就是本包的事，老版同）。
func TestInitCreatesHistoryDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "history")

	lg, err := Init(root, "SSHFleetTools.log")
	if err != nil {
		t.Fatalf("开工具日志失败：%v", err)
	}
	defer func() { _ = lg.Close() }()

	if _, err := os.Stat(filepath.Join(root, "SSHFleetTools.log")); err != nil {
		t.Fatalf("工具日志没落在那条路径上：%v", err)
	}
}
