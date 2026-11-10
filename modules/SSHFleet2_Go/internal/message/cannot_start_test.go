package message

import (
	"testing"

	"sshfleet2/internal/contract"
)

// 分类重名那两句（自定义区）：条目号从 1 起，句子只在这里成形。
//
// 两张表的句子只差前面那一截前缀——错误分类表是「错误分类关键词第 N 条…」，改密表是
// 「改密配置第 N 条 category…」；名字与两个条目号的位置一致，正是 dev 定的形状。
func TestCannotStartDuplicateCategoryText(t *testing.T) {
	cases := []struct {
		name string
		c    contract.CannotStart
		want string
	}{
		{
			name: "错误分类关键词同块重名",
			c: contract.CannotStart{Reason: contract.CannotStartReason{
				Kind:  contract.CannotStartKeywordCatDuplicate,
				Index: 1,
				Limit: 3,
				Value: "上传回滚未清干净",
			}},
			want: "错误分类关键词第 1 条与第 3 条重名：上传回滚未清干净",
		},
		{
			name: "改密配置 category 同块重名",
			c: contract.CannotStart{Reason: contract.CannotStartReason{
				Kind:  contract.CannotStartPasswdCatDuplicate,
				Place: "category",
				Index: 2,
				Limit: 4,
				Value: "改密成功",
			}},
			want: "改密配置第 2 条 category 与第 4 条重名：改密成功",
		},
	}
	for _, c := range cases {
		if got := CannotStartText(c.c); got != c.want {
			t.Fatalf("%s：\n got  %q\n want %q", c.name, got, c.want)
		}
	}
}
