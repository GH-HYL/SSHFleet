package message

import "testing"

// 凭据写相对名、配置没配 secret_dir 那一句（自定义区）：逐字照迁移前 `parse` 里那个 error 的字。
func TestCredentialRelativeWithoutSecretDir(t *testing.T) {
	want := "写的是相对路径，但配置里没配 credential.secret_dir（提示：在配置的 [credential] 段配上 secret_dir，或这一格改写本机绝对路径）"
	if got := CredentialRelativeWithoutSecretDir(); got != want {
		t.Fatalf("secret_dir 缺项那一句：\n got  %q\n want %q", got, want)
	}
}
