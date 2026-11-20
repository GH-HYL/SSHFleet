package parse

// 随包判据表的顺序：认领 `client_failed` 的那一类必须排在「权限不足」之前（自定义区）。
//
// 判据是「报文里会带着远端原话」：提权失败时 sudo 自己的 `Permission denied` 跟在
// `sshfleet.transfer.client_failed:` 后面，而关键词是子串匹配、块内先命中者胜出——
// 「权限不足」写在前面，这一整条就被它认走，分类名与报文说的不是一件事。

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestPackagedCategoryOrderPutsClientFailedFirst(t *testing.T) {
	// 判据表路径是随包写死的相对路径，没有注入点：切到模块根，让它按 ./config 找到。
	t.Chdir(filepath.Join("..", ".."))

	cats, _, fail := loadErrorKeywords()
	if fail != nil {
		t.Fatalf("随包判据表读不起来：%+v", *fail)
	}

	// 谁先认领：第一个关键词里含这一条的类，就是这条原文会落到的类。
	rank := func(keyword string) int {
		for i, c := range cats {
			if slices.Contains(c.Keywords, keyword) {
				return i
			}
		}
		return -1
	}

	clientFailed := rank("sshfleet.transfer.client_failed")
	denied := rank("permission denied")
	if clientFailed < 0 || denied < 0 {
		t.Fatalf("随包判据表里少了关键词：client_failed=%d，permission denied=%d", clientFailed, denied)
	}
	if clientFailed > denied {
		t.Fatalf("认领 client_failed 的那一类排在第 %d 位，晚于「权限不足」的第 %d 位——"+
			"带 Permission denied 的提权失败会被「权限不足」认走", clientFailed, denied)
	}
}
