package message

import "testing"

// 塔尖那四句「进度留痕」的逐字断言。
//
// 期望值是从**老版真跑出来的工具日志**里抄的（2026-11-11 的 A/B 现场），不是从代码推的：
// 「逐字照老版」这条判据只有拿老版的输出当基准才算数。三处标点各有讲究——
// 「生成归档目录成功: 」用 ASCII 冒号加空格，其余三句用全角冒号，老版就长这样。

func TestTraceArchiveCreatedWording(t *testing.T) {
	want := "生成归档目录成功: history/2026-11-11_09-18-37_passwd"
	if got := TraceArchiveCreated("history/2026-11-11_09-18-37_passwd").Plain(); got != want {
		t.Fatalf("\n要：%q\n得：%q", want, got)
	}
}

func TestTraceExecLogRotatedWording(t *testing.T) {
	const p = "history/2026-11-11_09-18-37_passwd/SSHFleetExec.log"
	want := "执行期日志已轮转至：" + p + "（执行结束自动切回）"
	if got := TraceExecLogRotated(p).Plain(); got != want {
		t.Fatalf("\n要：%q\n得：%q", want, got)
	}
}

func TestTraceExecLogClosedWording(t *testing.T) {
	const p = "history/2026-11-11_09-18-37_passwd/SSHFleetExec.log"
	want := "执行期日志已写完，切回工具日志：" + p
	if got := TraceExecLogClosed(p).Plain(); got != want {
		t.Fatalf("\n要：%q\n得：%q", want, got)
	}
}

func TestTraceExitWording(t *testing.T) {
	want := "SSHFleet已退出，日志文件：history/SSHFleetTools.log"
	if got := TraceExit("history/SSHFleetTools.log").Plain(); got != want {
		t.Fatalf("\n要：%q\n得：%q", want, got)
	}
}

// 四句都只进日志：不带色码、不带结尾换行（日志那一行自己加），否则日志文件里会多出转义字符。
func TestTraceLinesAreLogOnly(t *testing.T) {
	for name, line := range map[string]Line{
		"归档目录": TraceArchiveCreated("history/x"),
		"轮转":   TraceExecLogRotated("history/x/SSHFleetExec.log"),
		"切回":   TraceExecLogClosed("history/x/SSHFleetExec.log"),
		"已退出":  TraceExit("history/SSHFleetTools.log"),
	} {
		got := line.Plain()
		for i := 0; i < len(got); i++ {
			if got[i] == 0x1b {
				t.Fatalf("%s 里不该有色码：%q", name, got)
			}
		}
		if len(got) > 0 && got[len(got)-1] == '\n' {
			t.Fatalf("%s 不该带结尾换行：%q", name, got)
		}
		if line.Stderr() {
			t.Fatalf("%s 是纯日志行，不该声明写到标准错误", name)
		}
	}
}
