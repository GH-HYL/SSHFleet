package mask

// 清单的脱敏副本：原样搬一份进本次目录的 `assets/`，只把凭据那两列遮掉。
//
// 为什么不能原样复制：明文密码就写在清单第 4 列，复制一份等于把凭据又存了一份。为什么
// 又不整份抹掉：归档要能回答「这次对哪些机器做了什么」，IP、端口、用户名、私钥路径都得留着。
//
// 遮法与别处同一条（`Secret`）——整个工程只有一种遮出来的样子。

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// maskManifestLine 遮一行：数据行的第 3、5 段（第 4 列密码、第 6 列口令）走 `Secret`，
// 其余原样。表头、注释、认不出是节点的行一律不动——那是取值判定那头的事，不在这里猜。
func maskManifestLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return line
	}
	fields, err := csvFields(line)
	if err != nil || len(fields) < 4 || !isIPv4(strings.TrimSpace(fields[0])) {
		return line
	}
	maskCredentialFields(fields)
	return joinFields(fields)
}

// csvFields 按 CSV 规则拆一行（容忍引号里的逗号、变长列）。
func csvFields(line string) ([]string, error) {
	r := csv.NewReader(strings.NewReader(line))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	return r.Read()
}

// joinFields 按 CSV 规则拼回一行；拼不出来时原样交回，宁可留原文也不写成半截。
func joinFields(fields []string) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(fields); err != nil {
		return strings.Join(fields, ",")
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return strings.Join(fields, ",")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
