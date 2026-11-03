//go:build integration

package execute

// 对着真机器改密（15 号单）：分配 PTY、按提示词表认出远端在问什么、喂当前密码与新密码。
//
// 现场要一个**密码已过期**的普通账号（`chage -d 0`）。造法（宿主上跑一次即可）：
//
//	podman exec u1 sh -c 'useradd -m t1 && echo "t1:OldPass#2026" | chpasswd && chage -d 0 t1'
//
// 再给两个变量：`SSHFLEET_TEST_PASSWD_USER=t1`、`SSHFLEET_TEST_PASSWD_PASSWORD=OldPass#2026`。
// 用例每跑一次都会把这个账号的口令重置回上面那个值并重新置为过期，所以可以反复跑。

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// passwdNewPassword 是这次要改成的新口令。
const passwdNewPassword = "Fleet#New2026"

// passwdTarget 造一个「密码已过期」的节点，并把它的口令重置回来。
func passwdTarget(t *testing.T, admin contract.Node) (contract.Node, string) {
	t.Helper()
	user := integrationEnv(t, "SSHFLEET_TEST_PASSWD_USER")
	old := integrationEnv(t, "SSHFLEET_TEST_PASSWD_PASSWORD")

	reset := fmt.Sprintf("echo '%s:%s' | sudo chpasswd && sudo chage -d 0 %s", user, old, user)
	if item := remoteRun(t, admin, reset); item.ExitCode == nil || *item.ExitCode != 0 {
		t.Fatalf("把 %s 的口令重置成过期态失败：%+v", user, item)
	}
	node := contract.Node{IP: admin.IP, Port: admin.Port, User: user, Password: old}
	return node, old
}

// TestRunPasswdChangesPassword 过期账号改成新口令：干净收场、输出里抹掉两个口令、新口令真能用。
func TestRunPasswdChangesPassword(t *testing.T) {
	admin, _ := integrationTarget(t)
	node, old := passwdTarget(t, admin)

	p := planFor(t, []contract.Node{node}, "--change-password", passwdNewPassword)
	item := runOne(t, context.Background(), p)

	if !item.Connected {
		t.Fatalf("过期账号该连得上（连上之后才轮到改密），错误原文 %q", item.ErrorText)
	}
	if item.ErrorText != "" {
		t.Fatalf("干净改完不该有报错原文，实际 %q（输出：%s）", item.ErrorText, item.Output)
	}
	if item.TimedOut || item.Cancelled {
		t.Fatalf("干净改完却带着超时/取消标志：%+v", item)
	}
	// 下发的 `:` 是中间步骤：它自己的退出码进 Steps，不顶成结果退出码。
	if item.ExitCode != nil {
		t.Fatalf("改密没有目的命令，退出码那一格该空着，实际 %v", *item.ExitCode)
	}
	if len(item.Steps) != 1 || item.Steps[0].Name != contract.StepPasswd {
		t.Fatalf("该记一条改密的中间步骤，实际 %+v", item.Steps)
	}
	if code := item.Steps[0].ExitCode; code == nil || *code != 0 {
		t.Fatalf("中间步骤的退出码应是 0，实际 %+v", item.Steps[0])
	}
	// 两个口令都不许留在输出里：远端关回显的时机不由我们定。
	if strings.Contains(item.Output, passwdNewPassword) || strings.Contains(item.Output, old) {
		t.Fatalf("输出里露出了口令：%s", item.Output)
	}

	// 新口令真能用：换上新口令再跑一条命令，过得去才算改成了。
	after := planFor(t, []contract.Node{{
		IP: node.IP, Port: node.Port, User: node.User, Password: passwdNewPassword,
	}}, "-c", "echo changed-ok")
	if got := runOne(t, context.Background(), after); got.Output != "changed-ok" {
		t.Fatalf("新口令登不进去（原文 %q，输出 %q）", got.ErrorText, got.Output)
	}
}

// TestRunPasswdNotExpired 账号没过期：这台不用改，收尾照实说「没见到过期提示」。
//
// 这一条同时钉住方向：改密把「密码过期」当**入场信号**，带上中止词会在第 0 秒把自己杀掉。
func TestRunPasswdNotExpired(t *testing.T) {
	node, _ := integrationTarget(t)

	p := planFor(t, []contract.Node{node}, "--change-password", passwdNewPassword)
	item := runOne(t, context.Background(), p)

	if !strings.Contains(item.ErrorText, "sshfleet.passwd.not_expired: ") {
		t.Fatalf("没过期的账号该说「没见到过期提示」，实际 %q", item.ErrorText)
	}
	// 分类名要认成改密那张表里的「密码未过期」，不是回落成上面那行报文。
	if item.Category != "密码未过期" {
		t.Fatalf("分类该是「密码未过期」，实际 %q", item.Category)
	}
	if item.TimedOut {
		t.Fatalf("`:` 跑完就该收场，不该等到超时（耗时 %v）", item.Cost)
	}
}
