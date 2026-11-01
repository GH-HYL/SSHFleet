package parse

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// 读配置：把 ./config/SSHFleet.conf 读成「节 → 键 → 原文值」。
//
// 本阶段只到这一步——类型化、必填校验、未知字段、路径规则随阶段 03；这里只取
// 帮助要用的 [execution] 四格。读不到、读不懂都算「起不来」（过渡形态，话术随 03 定稿）。

const (
	configPath      = "./config/SSHFleet.conf"
	shownConfigPath = "config/SSHFleet.conf" // 报错里展示的形态：不带 ./，它不是路径的一部分
)

// rawConfig 配置原文表：节名 → （键名 → 原文值）。节名含点号节（[a.b]）与数组表（[[x]]）。
type rawConfig map[string]map[string]string

// readRawConfig 读当前目录下的配置文件。文件不在按老版同一句话说清「该在哪、现在在哪」。
func readRawConfig() (rawConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, missingConfigError()
	}
	return parseRawConfig(data)
}

// missingConfigError 对位老实现 config.Load 的缺失文案（系统原话那句不翻新）。
func missingConfigError() error {
	wd, _ := os.Getwd()
	return fmt.Errorf(
		"这个位置没有配置文件\n"+
			"      应该在这里：%s\n"+
			"      当前目录：%s\n"+
			"提示：请在本工具的解压目录下执行；配置文件必须放在该目录的 config 文件夹里",
		shownConfigPath, wd)
}

// parseRawConfig 把配置读成原文表。只认这六份配置真用到的形态：
//
//   - [节] 与 [[节]]（数组表，键名集合里算同一节）、[a.b] 点号节——节名原样收；
//   - 键 = 值：'…' 或 "…" 字符串、整数、布尔；引号里的 # 不是注释；
//   - 跨行数组（键 = [ 换行 … ]）整段收掉，键名照记——判据表那些 keywords 就是这种；
//   - # 注释与空行跳过。
//
// 引号外的转义不展开、重复键后值盖前值——出厂配置里没有这两种形态，有的话阶段 03 校验会拦。
func parseRawConfig(data []byte) (rawConfig, error) {
	raw := rawConfig{}
	section := ""
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(stripComment(lines[i]))
		if trimmed == "" {
			continue
		}

		if name, ok := sectionName(trimmed); ok {
			if name == "" {
				return nil, fmt.Errorf("第 %d 行的节名是空的", i+1)
			}
			section = name
			if raw[section] == nil {
				raw[section] = map[string]string{}
			}
			continue
		}

		eq := strings.Index(trimmed, "=")
		if eq < 0 {
			return nil, fmt.Errorf("第 %d 行既不是节也不是键值对：%s", i+1, trimmed)
		}
		key := strings.TrimSpace(trimmed[:eq])
		if key == "" {
			return nil, fmt.Errorf("第 %d 行的键名是空的", i+1)
		}
		if section == "" {
			return nil, fmt.Errorf("第 %d 行的键 %s 出现在任何节之前", i+1, key)
		}

		value := strings.TrimSpace(trimmed[eq+1:])
		// 跨行数组：以 [ 开头还没见到配对的 ]，就一直吃到出现 ] 为止
		for strings.HasPrefix(value, "[") && !strings.Contains(value, "]") && i+1 < len(lines) {
			i++
			value += " " + strings.TrimSpace(stripComment(lines[i]))
		}

		if raw[section] == nil {
			raw[section] = map[string]string{}
		}
		raw[section][key] = unquote(value)
	}
	return raw, nil
}

// sectionName 认节行：[名] 与 [[名]]（数组表）都算节，返回里层那个名字。
func sectionName(trimmed string) (string, bool) {
	switch {
	case strings.HasPrefix(trimmed, "[[") && strings.HasSuffix(trimmed, "]]"):
		return strings.TrimSpace(trimmed[2 : len(trimmed)-2]), true
	case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"):
		return strings.TrimSpace(trimmed[1 : len(trimmed)-1]), true
	}
	return "", false
}

// stripComment 裁掉引号外的 # 注释：引号里出现 # 是值的一部分（密码里常有）。
func stripComment(line string) string {
	var quote rune
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '#':
			return line[:i]
		}
	}
	return line
}

// unquote 去掉成对的外层引号（'…' 或 "…"），内部原样留着。
func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '\'' && v[len(v)-1] == '\'') || (v[0] == '"' && v[len(v)-1] == '"') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

// execution 是帮助要用的 [execution] 四格。
type execution struct {
	Sudo            bool
	TimeoutConnect  int
	TimeoutExecute  int
	TimeoutTransfer int
}

// executionOf 从原文表取四格。取不到、认不出都是「起不来」——过渡形态，随阶段 03 并进校验。
func executionOf(raw rawConfig) (execution, error) {
	sec, ok := raw["execution"]
	if !ok {
		return execution{}, fmt.Errorf("配置里没有 [execution] 段")
	}
	var out execution

	sudo, err := grab(sec, "sudo")
	if err != nil {
		return execution{}, err
	}
	if out.Sudo, err = strconv.ParseBool(sudo); err != nil {
		return execution{}, fmt.Errorf("execution.sudo 取值非法：%s（只认 true / false）", sudo)
	}

	if out.TimeoutConnect, err = grabInt(sec, "timeout_connect"); err != nil {
		return execution{}, err
	}
	if out.TimeoutExecute, err = grabInt(sec, "timeout_execute"); err != nil {
		return execution{}, err
	}
	if out.TimeoutTransfer, err = grabInt(sec, "timeout_transfer"); err != nil {
		return execution{}, err
	}
	return out, nil
}

func grab(sec map[string]string, key string) (string, error) {
	v, ok := sec[key]
	if !ok || v == "" {
		return "", fmt.Errorf("execution.%s 缺失或为空", key)
	}
	return v, nil
}

func grabInt(sec map[string]string, key string) (int, error) {
	v, err := grab(sec, key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("execution.%s 取值非法：%s（须为整数，单位秒）", key, v)
	}
	return n, nil
}
