package message

import (
	"testing"

	"sshfleet2/internal/contract"
)

// 中间步骤那一行：有一步非 0 就整段列出（含那几步的 0），全 0 或没有则一个字都不写。
func TestStepNote(t *testing.T) {
	code0, code1 := 0, 1
	got := StepNote([]contract.Step{
		{Name: contract.StepPrecheck, ExitCode: &code1},
		{Name: contract.StepEnumerate, ExitCode: &code0},
	})
	want := "中间步骤：下载预检（test -e） 退出码 1、远程文件枚举（find） 退出码 0"
	if got != want {
		t.Fatalf("got=%q\nwant=%q", got, want)
	}

	if got := StepNote([]contract.Step{{Name: contract.StepPasswd, ExitCode: &code0}}); got != "" {
		t.Fatalf("全 0 不该给话：%q", got)
	}
	if got := StepNote(nil); got != "" {
		t.Fatalf("没有中间步骤不该给话：%q", got)
	}
}

// 拿不到退出码（超时、中断）时如实写「未拿到」，不推 0。
func TestStepNoteNoExitCode(t *testing.T) {
	code1 := 1
	got := StepNote([]contract.Step{
		{Name: contract.StepPasswd, ExitCode: &code1},
		{Name: contract.StepPrecheck},
	})
	want := "中间步骤：改密收尾（:） 退出码 1、下载预检（test -e） 未拿到退出码"
	if got != want {
		t.Fatalf("got=%q\nwant=%q", got, want)
	}
}
