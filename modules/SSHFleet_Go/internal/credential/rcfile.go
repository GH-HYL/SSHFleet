// rc 文件（登录 shell 启动脚本）里主密钥行的读写原语。
//
// 不挂平台标签：这里是纯文本处理，抽出来是为了让单测在任一平台都能跑
// （开发机是 Windows，Linux 侧逻辑仍需可验证）。Unix 侧 masterkey_unix.go
// 负责「选哪个 rc 文件、要不要同步另一侧」这类平台相关决策。
package credential

import (
	"os"
	"strings"
)

// keyComment 本工具在 rc 文件里写下的分组注释行（重写时一并清理，避免堆积）。
const keyComment = "# SSHFleet 主密钥"

// isKeyExportLine 判定一行是否为本工具格式的主密钥赋值行。
// 只认规范形态（trim 后以 export 开头）：注释掉的行（# 开头）不算，
// 避免把用户备注里的示例误当成生效值。
func isKeyExportLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "export "+envName+"=")
}

// isKeyGapComment 判定一行是否为本工具写下的分组注释。
func isKeyGapComment(line string) bool {
	return strings.TrimSpace(line) == keyComment
}

// readLines 按行读文件；文件不存在或读失败返回 nil。
func readLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(string(data), "\n")
}

// hasKeyLine rc 文件里是否已有本工具格式的主密钥行（不存在/无则 false）。
func hasKeyLine(path string) bool {
	for _, line := range readLines(path) {
		if isKeyExportLine(line) {
			return true
		}
	}
	return false
}

// findExportInFile 在 rc 文件里找 export SSHFLEET_KEY=... 的值；多条时取**最后一条**。
//
// 取最后一条是 2026-09-15 的修复：shell 以最后一条赋值生效，原先取第一条与
// shell 语义相反，rc 里存在两条时会与终端实际生效值分叉。
func findExportInFile(path string) string {
	needle := "export " + envName + "="
	value := ""
	for _, line := range readLines(path) {
		if !isKeyExportLine(line) {
			continue
		}
		// 对位旧正则 export SSHFLEET_KEY=['"]?([^'"\s]+)：取引号或空白前的值
		rest := line[strings.Index(line, needle)+len(needle):]
		rest = strings.TrimLeft(rest, "'\"")
		if i := strings.IndexAny(rest, "'\"\t "); i >= 0 {
			rest = rest[:i]
		}
		value = rest
	}
	return value
}

// writeKeyToRC 把 rc 文件里的主密钥收敛为**末尾唯一一条** export 行：
// 清掉全部主密钥赋值行与本工具的分组注释，在文件末尾追加一组。
//
// 2026-09-15 修：原先只替换文件里**第一条** export 行，而 shell 以**最后一条**
// 生效——rc 里存在两条时（手动加过一条、工具又写过一次）就会出现「工具认为密钥是 X、
// 终端实际用 Y」的分叉，症状是凭据解密报「主密钥不匹配」。放在末尾可保证
// 「工具读到的」＝「终端实际用的」。文件不存在时按新建处理（不额外留空行）。
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
	kept = append(kept, keyComment, "export "+envName+"='"+key+"'")
	return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
}
