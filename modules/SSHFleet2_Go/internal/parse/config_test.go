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
	lines, err := tokenize(data)
	if err != nil {
		t.Fatalf("样本 %s 解析失败：%v", name, err)
	}
	return rawView(lines)
}

func TestParseRawConfigShapes(t *testing.T) {
	raw := sample(t, "normal.conf")
	if raw["execution"]["sudo"] != "true" {
		t.Errorf("布尔原文：%q", raw["execution"]["sudo"])
	}
	if raw["execution"]["timeout_transfer"] != "300" {
		t.Errorf("整数原文：%q", raw["execution"]["timeout_transfer"])
	}

	// 引号里的 # 不是注释：密码不能被截短。原文表存的是**原文**，引号留在里面
	// （字符串要不要引号是 stringOf 按类型判的事）。
	raw = sample(t, "hash_in_value.conf")
	if got := raw["account"]["password"]; got != "\"pw#123\"" {
		t.Errorf("值里的 # 被当注释截断了：got %q", got)
	}

	// 点号节原样当节名，不碍着别的节
	raw = sample(t, "dotted.conf")
	if raw["interpreter.script"][".sh"] == "" {
		t.Errorf("点号节的键应原样收下（外层引号去掉）：%v", raw["interpreter.script"])
	}
	if raw["interpreter"]["command"] != "\"bash\"" {
		t.Errorf("点号节不碍着普通节：%v", raw["interpreter"])
	}

	// 单引号字符串照老配置的写法
	raw = sample(t, "single_quote.conf")
	if raw["category"]["name"] != "'上传回滚未清干净'" {
		t.Errorf("单引号值：%q", raw["category"]["name"])
	}

	// 跨行数组整段收掉，键名照记
	raw = sample(t, "multiline_array.conf")
	if raw["category"]["keywords"] == "" {
		t.Errorf("跨行数组的键名应照记：%v", raw["category"])
	}
}

func TestParseRawConfigRejects(t *testing.T) {
	if _, err := tokenize([]byte("孤零零一行\n")); err == nil {
		t.Errorf("既不是节也不是键值对的行应报错")
	}
	if _, err := tokenize([]byte("key = \"v\"\n")); err == nil {
		t.Errorf("键出现在任何节之前应报错")
	}
	if _, err := tokenize([]byte("[   ]\n")); err == nil {
		t.Errorf("空节名应报错")
	}
}

// 同节同名键写两次：当场报错，不静默取后值（2026-11-10，审计 06-H3）。
// 注释一直写着「遇到就报错」，代码却是后值盖前值、还退 0——这里把注释兑现。
func TestParseRejectsDuplicateKeys(t *testing.T) {
	_, err := tokenize([]byte("[account]\nport = 22\nport = 2201\n"))
	if err == nil {
		t.Fatal("同节同名键写两次应当报错，不取后值")
	}
	if !strings.Contains(err.Error(), "写了两次") {
		t.Fatalf("报错该说清哪个键写了两次：%v", err)
	}

	// 数组表各条目键名当然重复：`[[category]]` 两条各有 name，那不叫冲突。
	arr := "[[category]]\nname = \"a\"\n[[category]]\nname = \"b\"\n"
	if _, err := tokenize([]byte(arr)); err != nil {
		t.Fatalf("数组表各条目的同名键不该报错：%v", err)
	}

	// 不同节的同名键也不冲突。
	other := "[account]\nport = 22\n[execution]\nport = 2201\n"
	if _, err := tokenize([]byte(other)); err != nil {
		t.Fatalf("不同节的同名键不该报错：%v", err)
	}
}

// 字符串键必须带引号：裸值（password = /tmp/x）该被拒，带引号的原样取出。
// 老版用 TOML 库，裸值当场是语法错——这条口径不能松。
func TestStringOfRequiresQuotes(t *testing.T) {
	raw := rawConfig{"account": {"password": "/tmp/pw"}}
	if _, err := stringOf(raw, "account", "password"); err == nil {
		t.Errorf("裸字符串值该被拒")
	}
	raw = rawConfig{"account": {"password": "\"/tmp/pw\""}}
	v, err := stringOf(raw, "account", "password")
	if err != nil || v != "/tmp/pw" {
		t.Errorf("带引号的值该取出引号里的内容：got %q, err %v", v, err)
	}
}

// execFrom 只从原文表取 [execution] 四格：样本文件是片段，不该逼它凑齐整份配置。
func execFrom(raw rawConfig) (execution, error) {
	var f execution
	var err error
	if f.Sudo, err = boolOf(raw, "execution", "sudo"); err != nil {
		return execution{}, err
	}
	if f.TimeoutConnect, err = intOf(raw, "execution", "timeout_connect"); err != nil {
		return execution{}, err
	}
	if f.TimeoutExecute, err = intOf(raw, "execution", "timeout_execute"); err != nil {
		return execution{}, err
	}
	if f.TimeoutTransfer, err = intOf(raw, "execution", "timeout_transfer"); err != nil {
		return execution{}, err
	}
	return f, nil
}

func TestExecutionOf(t *testing.T) {
	f, err := execFrom(sample(t, "normal.conf"))
	if err != nil {
		t.Fatalf("四格应取得到：%v", err)
	}
	// 期望值写死（随包配置的出厂值），不用被测代码算
	if f.Sudo != true || f.TimeoutConnect != 10 || f.TimeoutExecute != 60 || f.TimeoutTransfer != 300 {
		t.Errorf("四格与出厂配置不符：%+v", f)
	}

	// 缺 [execution] 段：起不来
	if _, err := execFrom(sample(t, "no_execution.conf")); err == nil {
		t.Errorf("缺 [execution] 段应报错")
	}

	// 取值非法：起不来
	raw := sample(t, "normal.conf")
	raw["execution"]["timeout_execute"] = "abc"
	if _, err := execFrom(raw); err == nil {
		t.Errorf("非整数应报错")
	}
	raw["execution"]["sudo"] = "maybe"
	if _, err := execFrom(raw); err == nil {
		t.Errorf("非布尔应报错")
	}
}

// 真配置：切换到模块根，读的就是随包那份（里面是出厂值）。
func TestReadRealConfig(t *testing.T) {
	writeTempConfig(t)
	cfg, err := load()
	if err != nil {
		t.Fatalf("真配置应读得到：%v", err)
	}
	f := cfg.Execution
	if f.Sudo != true || f.TimeoutConnect != 10 || f.TimeoutExecute != 60 || f.TimeoutTransfer != 300 {
		t.Errorf("真配置四格与出厂值不符：%+v", f)
	}
}

// 真配置不在（切到一个空目录）：readRawConfig 报「这个位置没有配置文件」那套话。
func TestReadMissingConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := load()
	if err == nil {
		t.Fatalf("空目录下应读不到配置")
	}
	for _, want := range []string{"这个位置没有配置文件", shownConfigPath, "当前目录："} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("缺失文案缺一句 %q：\n%s", want, err.Error())
		}
	}
}
