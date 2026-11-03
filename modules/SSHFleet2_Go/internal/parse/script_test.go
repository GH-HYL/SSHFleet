package parse

// 脚本模式的解释器与正文整备（建站 05 的 04 号单）。
//
// 走真配置、真临时文件：解释器表是配置里那张映射，正文来自磁盘上的脚本文件——
// 直接调一个纯函数验不出「读一次、整备一次」这件事。

import (
	"os"
	"path/filepath"
	"testing"
)

// 后缀命中：用它配的那个解释器，纸上不留「回退后缀」。
func TestInterpreterBySuffixHit(t *testing.T) {
	dir := writeTempConfig(t)
	manifest := writeFile(t, dir, "ok.csv", "10.0.0.1,22,root,pw\n")
	script := writeFile(t, dir, "deploy.sh", "echo hi\n")

	cfg, err := load()
	if err != nil {
		t.Fatalf("临时配置读不通：%v", err)
	}
	p, fail := buildPlan(cfg, scan([]string{"sshfleet2", "-f", manifest, "-s", script}))
	if fail != nil {
		t.Fatalf("不该走到「起不来」：%v", fail.Body)
	}
	if p.Run.Interpreter != "bash" {
		t.Fatalf("`.sh` 该配到 bash，实际 %q", p.Run.Interpreter)
	}
	if p.Run.FallbackExt != "" {
		t.Fatalf("配到了就不该留回退后缀，实际 %q", p.Run.FallbackExt)
	}
}

// 后缀大小写不区分：`.SH` 与 `.sh` 同命。
func TestInterpreterSuffixIsCaseInsensitive(t *testing.T) {
	dir := writeTempConfig(t)
	manifest := writeFile(t, dir, "ok.csv", "10.0.0.1,22,root,pw\n")
	script := writeFile(t, dir, "DEPLOY.SH", "echo hi\n")

	cfg, err := load()
	if err != nil {
		t.Fatalf("临时配置读不通：%v", err)
	}
	p, fail := buildPlan(cfg, scan([]string{"sshfleet2", "-f", manifest, "-s", script}))
	if fail != nil {
		t.Fatalf("不该走到「起不来」：%v", fail.Body)
	}
	if p.Run.Interpreter != "bash" || p.Run.FallbackExt != "" {
		t.Fatalf("`.SH` 应当命中 `.sh`：解释器 %q，回退后缀 %q", p.Run.Interpreter, p.Run.FallbackExt)
	}
}

// 后缀没配到：回退到命令解释器，并把那个后缀记在纸上（闸门据此问一句）。
func TestInterpreterFallsBackAndRecordsSuffix(t *testing.T) {
	dir := writeTempConfig(t)
	manifest := writeFile(t, dir, "ok.csv", "10.0.0.1,22,root,pw\n")
	script := writeFile(t, dir, "install.bin", "echo hi\n")

	cfg, err := load()
	if err != nil {
		t.Fatalf("临时配置读不通：%v", err)
	}
	p, fail := buildPlan(cfg, scan([]string{"sshfleet2", "-f", manifest, "-s", script}))
	if fail != nil {
		t.Fatalf("不该走到「起不来」：%v", fail.Body)
	}
	if p.Run.Interpreter != "bash" {
		t.Fatalf("没配到应当回退到命令解释器 bash，实际 %q", p.Run.Interpreter)
	}
	if p.Run.FallbackExt != ".bin" {
		t.Fatalf("回退后缀应当是 .bin，实际 %q", p.Run.FallbackExt)
	}
}

// 正文整备只做一次：剥 BOM、去整块首尾空白。
//
// 量到的那份与实际要发的是同一个值——下发行那一边取的是纸上这一格。
func TestScriptBodyTrimmedOnce(t *testing.T) {
	dir := writeTempConfig(t)
	manifest := writeFile(t, dir, "ok.csv", "10.0.0.1,22,root,pw\n")
	script := filepath.Join(dir, "deploy.sh")
	if err := os.WriteFile(script, []byte("\ufeff\n\n  echo hi\n\n"), 0o644); err != nil {
		t.Fatalf("造脚本失败：%v", err)
	}

	cfg, err := load()
	if err != nil {
		t.Fatalf("临时配置读不通：%v", err)
	}
	p, fail := buildPlan(cfg, scan([]string{"sshfleet2", "-f", manifest, "-s", script}))
	if fail != nil {
		t.Fatalf("不该走到「起不来」：%v", fail.Body)
	}
	if p.Task.Script.Body != "echo hi" {
		t.Fatalf("正文整备不对：%q", p.Task.Script.Body)
	}
	if p.Run.Stdin != p.Task.Script.Body {
		t.Fatalf("要发的正文与纸上那份不是同一个值：%q vs %q", p.Run.Stdin, p.Task.Script.Body)
	}
}

// 命令模式仍取命令解释器；上传、下载、改密三支的解释器为空。
func TestInterpreterOnlyForCommandAndScript(t *testing.T) {
	dir := writeTempConfig(t)
	manifest := writeFile(t, dir, "ok.csv", "10.0.0.1,22,root,pw\n")
	target := filepath.Join(dir, "logs")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("造目录失败：%v", err)
	}
	upload := writeFile(t, dir, "a.txt", "x")

	cfg, err := load()
	if err != nil {
		t.Fatalf("临时配置读不通：%v", err)
	}
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{"命令模式", []string{"-c", "uptime"}, "bash"},
		{"上传", []string{"-u", upload, "-p", "/tmp/"}, ""},
		{"下载", []string{"-d", "/tmp/x", "-p", target}, ""},
		{"改密", []string{"--change-password", "pw"}, ""},
	}
	for _, c := range cases {
		p, fail := buildPlan(cfg, scan(append([]string{"sshfleet2", "-f", manifest}, c.argv...)))
		if fail != nil {
			t.Fatalf("%s 不该走到「起不来」：%v", c.name, fail.Body)
		}
		if p.Run.Interpreter != c.want {
			t.Errorf("%s 的解释器应当是 %q，实际 %q", c.name, c.want, p.Run.Interpreter)
		}
	}
}
