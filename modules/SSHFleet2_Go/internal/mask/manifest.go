package mask

// 清单的脱敏副本：原样搬一份进本次目录的 `assets/`，只把凭据那两列遮掉。
//
// 为什么不能原样复制：明文密码就写在清单第 4 列，复制一份等于把凭据又存了一份。为什么
// 又不整份抹掉：归档要能回答「这次对哪些机器做了什么」，IP、端口、用户名、私钥路径都得留着。
//
// 遮法与别处同一条（`Secret`）——整个工程只有一种遮出来的样子。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshfleet2/internal/criterion"
)

// WriteManifestCopy 把清单按脱敏副本写进 destDir，文件名不变。
//
// 三种情况什么都不做（照老版）：路径空着（这次用的是内联记录）、不是一个普通文件、
// 读不出来。内联记录没有文件可备份——它已经随 `report.txt` 那一段遮过一遍了。
//
// 逐行处理，行序、空行、注释行、引号原样保留：用户拿副本当清单再跑一次也要能跑。
func WriteManifestCopy(src, destDir string) error {
	if src == "" {
		return nil
	}
	info, err := os.Stat(src)
	if err != nil || info.IsDir() {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("读不到清单 %s：%w", src, err)
	}

	var out strings.Builder
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		out.WriteString(maskManifestLine(line))
		out.WriteString("\n")
	}

	dst := filepath.Join(destDir, filepath.Base(src))
	if err := os.WriteFile(dst, []byte(out.String()), info.Mode().Perm()); err != nil {
		return fmt.Errorf("写不进清单副本 %s：%w", dst, err)
	}
	return nil
}

// maskManifestLine 遮一行：只替换凭据那两段（第 4 列密码、第 6 列口令）在**原行**里占的那几个
// 字符，其余字节一个都不动——副本要像 `cp` 出来的一样，BOM、引号、空白、列序统统照原样。
//
// 为什么不再按 CSV 读出来再拼回去：`encoding/csv` 读的时候会吃掉 BOM，写的时候按它自己的
// 规矩决定加不加引号，拼出来那一行跟原件对不上（BOM 没了、原本带引号的变成不带）。
// 遮罩不该改一个字的其余部分，所以这里改成定位区间、就地替换。
//
// 表头、注释、认不出是节点的行一律不动——那是取值判定那头的事，不在这里猜。
func maskManifestLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return line
	}
	spans, ok := fieldSpans(line)
	if !ok || len(spans) < 4 {
		return line
	}
	// 首列是不是 IP：BOM 不算内容，只在判断时剥掉，输出里照旧留着。
	first := strings.TrimSpace(strings.TrimPrefix(line[spans[0].start:spans[0].end], "\ufeff"))
	if !criterion.IsIPv4(first) {
		return line
	}

	out := line
	// 从右往左替换：先动后面的段，前面那些段的偏移才不会被前一次替换搅乱。
	for i := len(spans) - 1; i >= 0; i-- {
		if i != credentialPassword && i != credentialPassphrase {
			continue
		}
		sp := spans[i]
		raw := out[sp.start:sp.end]
		if strings.TrimSpace(raw) == "" {
			continue
		}
		out = out[:sp.start] + Secret(raw) + out[sp.end:]
	}
	return out
}
