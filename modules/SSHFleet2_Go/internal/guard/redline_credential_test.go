package guard

import (
	"testing"

	"sshfleet2/internal/credential"
)

// 红线：凭据包的那一件事——报主密钥齐不齐、把一段凭据换成明文。
//
// 只钉形状，不钉值：状态只有两种、入口在空值上不炸、不自己起进程或打屏（后两条由
// 类一「写终端」与类三「退出动作」静态盯着）。密文解得对不对是自定义区的事。
func TestRedlineCredentialReportsState(t *testing.T) {
	switch st := credential.Status(); st {
	case credential.KeyReady, credential.KeyMissing:
	default:
		t.Fatalf("主密钥状态只许是 ready 或 missing，拿到 %q", st)
	}

	// 加密关着时那一格就是密码本身，这条路径不碰环境变量、不碰磁盘。
	if _, err := credential.Secret("", false); err != nil {
		t.Fatalf("加密关着时不该报错：%v", err)
	}
}
