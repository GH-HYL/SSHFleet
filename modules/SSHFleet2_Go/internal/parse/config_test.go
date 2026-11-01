package parse

import (
	"os"
	"strings"
	"testing"
)

func sample(t *testing.T, name string) rawConfig {
	t.Helper()
	data, err := os.ReadFile("testdata/conf_samples/" + name)
	if err != nil {
		t.Fatalf("读样本 %s 失败：%v", name, err)
	}
	raw, err := parseRawConfig(data)
	if err != nil {
		t.Fatalf("样本 %s 解析失败：%v", name, err)
	}
	return raw
}

func TestParseRawConfigShapes(t *testing.T) {
	raw := sample(t, "normal.conf")
	if raw["execution"]["sudo"] != "true" {
		t.Errorf("布尔原文：%q", raw["execution"]["sudo"])
	}
	if raw["execution"]["timeout_transfer"] != "300" {
		t.Errorf("整数原文：%q", raw["execution"]["timeout_transfer"])
	}

	// 引号里的 # 不是注释：密码不能被截短
	raw = sample(t, "hash_in_value.conf")
	if got := raw["account"]["password"]; got != "pw#123" {
		t.Errorf("值里的 # 被当注释截断了：got %q", got)
	}

	// 点号节原样当节名，不碍着别的节
	raw = sample(t, "dotted.conf")
	if raw["interpreter.script"]["\".sh\""] == "" {
		t.Errorf("点号节的键应原样收下：%v", raw["interpreter.script"])
	}
	if raw["interpreter"]["command"] != "bash" {
		t.Errorf("点号节不碍着普通节：%v", raw["interpreter"])
	}

	// 单引号字符串照老配置的写法
	raw = sample(t, "single_quote.conf")
	if raw["categories"]["name"] != "上传回滚未清干净" {
		t.Errorf("单引号值：%q", raw["categories"]["name"])
	}

	// 跨行数组整段收掉，键名照记
	raw = sample(t, "multiline_array.conf")
	if raw["categories"]["keywords"] == "" {
		t.Errorf("跨行数组的键名应照记：%v", raw["categories"])
	}
}

func TestParseRawConfigRejects(t *testing.T) {
	if _, err := parseRawConfig([]byte("孤零零一行\n")); err == nil {
		t.Errorf("既不是节也不是键值对的行应报错")
	}
	if _, err := parseRawConfig([]byte("key = \"v\"\n")); err == nil {
		t.Errorf("键出现在任何节之前应报错")
	}
	if _, err := parseRawConfig([]byte("[   ]\n")); err == nil {
		t.Errorf("空节名应报错")
	}
}

func TestExecutionOf(t *testing.T) {
	f, err := executionOf(sample(t, "normal.conf"))
	if err != nil {
		t.Fatalf("四格应取得到：%v", err)
	}
	// 期望值写死（随包配置的出厂值），不用被测代码算
	if f.Sudo != true || f.TimeoutConnect != 10 || f.TimeoutExecute != 60 || f.TimeoutTransfer != 300 {
		t.Errorf("四格与出厂配置不符：%+v", f)
	}

	// 缺 [execution] 段：起不来
	if _, err := executionOf(sample(t, "no_execution.conf")); err == nil {
		t.Errorf("缺 [execution] 段应报错")
	}

	// 取值非法：起不来
	raw := sample(t, "normal.conf")
	raw["execution"]["timeout_execute"] = "abc"
	if _, err := executionOf(raw); err == nil {
		t.Errorf("非整数应报错")
	}
	raw["execution"]["sudo"] = "maybe"
	if _, err := executionOf(raw); err == nil {
		t.Errorf("非布尔应报错")
	}
}

// 真配置：切换到模块根，读的就是随包那份（里面是出厂值）。
func TestReadRealConfig(t *testing.T) {
	t.Chdir("../..")
	raw, err := readRawConfig()
	if err != nil {
		t.Fatalf("真配置应读得到：%v", err)
	}
	f, err := executionOf(raw)
	if err != nil {
		t.Fatalf("真配置四格应取得到：%v", err)
	}
	if f.Sudo != true || f.TimeoutConnect != 10 || f.TimeoutExecute != 60 || f.TimeoutTransfer != 300 {
		t.Errorf("真配置四格与出厂值不符：%+v", f)
	}
}

// 真配置不在（切到一个空目录）：readRawConfig 报「这个位置没有配置文件」那套话。
func TestReadMissingConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := readRawConfig()
	if err == nil {
		t.Fatalf("空目录下应读不到配置")
	}
	for _, want := range []string{"这个位置没有配置文件", shownConfigPath, "当前目录："} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("缺失文案缺一句 %q：\n%s", want, err.Error())
		}
	}
}
