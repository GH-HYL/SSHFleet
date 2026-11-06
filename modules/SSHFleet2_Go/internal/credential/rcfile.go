package credential

import (
	"os"
	"strings"
)

// 登录 shell 启动脚本（rc 文件）里主密钥行的读写原语。
//
// 不挂平台标签：这里是纯文本处理，抽出来是为了让两侧的单测都能跑。挑哪个 rc 文件、
// 要不要同步另一侧，是 persist_posix.go 的事。

// keyComment 是本工具在 rc 文件里写下的分组注释（重写时一并清理，免得越堆越多）。
const keyComment = "# SSHFleet 主密钥"

// isKeyExportLine 判一行是不是本工具格式的主密钥赋值行。
//
// 只认规范形态（trim 后以 `export SSHFLEET_KEY=` 开头）：注释掉的行不算，免得把使用者
// 备注里的示例当成生效值。
func isKeyExportLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "export "+EnvName+"=")
}

// isKeyGapComment 判一行是不是本工具写下的分组注释。
func isKeyGapComment(line string) bool {
	return strings.TrimSpace(line) == keyComment
}

// readLines 按行读文件；文件不存在或读失败交回 nil。
func readLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(string(data), "\n")
}

// hasKeyLine 这个 rc 文件里有没有本工具格式的主密钥行。
func hasKeyLine(path string) bool {
	for _, line := range readLines(path) {
		if isKeyExportLine(line) {
			return true
		}
	}
	return false
}

// findExportInFile 在 rc 文件里找 `export SSHFLEET_KEY=…` 的值；多条时取**最后一条**。
//
// 取最后一条是硬要求：shell 以最后一条赋值生效，取第一条会与终端实际生效值分叉，
// 症状就是「凭据解密报主密钥不匹配」。
func findExportInFile(path string) string {
	needle := "export " + EnvName + "="
	value := ""
	for _, line := range readLines(path) {
		if !isKeyExportLine(line) {
			continue
		}
		rest := line[strings.Index(line, needle)+len(needle):]
		rest = strings.TrimLeft(rest, "'\"")
		if i := strings.IndexAny(rest, "'\"\t "); i >= 0 {
			rest = rest[:i]
		}
		value = rest
	}
	return value
}

// writeKeyToRC 把 rc 文件里的主密钥收敛为**末尾唯一一条** export 行。
//
// 清掉全部主密钥赋值行与本工具的分组注释，再在末尾追加一组。写末尾是为了让「工具读到的」
// 永远等于「终端实际用的」——shell 以最后一条生效。
func writeKeyToRC(path, key string) error {
	kept := make([]string, 0, 16)
	for _, line := range readLines(path) {
		if isKeyExportLine(line) || isKeyGapComment(line) {
			continue
		}
		kept = append(kept, line)
	}
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	if len(kept) > 0 {
		kept = append(kept, "")
	}
	kept = append(kept, keyComment, "export "+EnvName+"='"+key+"'")
	return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
}
