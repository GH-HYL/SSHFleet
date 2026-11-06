package mask

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 清单脱敏副本：只遮第 4、6 列，注释与表头原样，行序与列数不变——副本要能直接当清单再跑。
func TestWriteManifestCopyMasksCredentialColumns(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nodes.csv")
	body := "# 生产环境\n" +
		"ip,port,user,password,key,key_password\n" +
		"10.0.0.1,22,root,hunter2,/k/id_rsa,sekretpass\n" +
		"10.0.0.2,22,root,pw,,\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatalf("造清单失败：%v", err)
	}

	assets := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatalf("建 assets 失败：%v", err)
	}
	if err := WriteManifestCopy(src, assets); err != nil {
		t.Fatalf("写副本失败：%v", err)
	}

	data, err := os.ReadFile(filepath.Join(assets, "nodes.csv"))
	if err != nil {
		t.Fatalf("副本没落下：%v", err)
	}
	// 末尾多一个换行是老版的口径（它按行拼、源文件本身以换行结尾，就多出一行空行）——
	// 这里照搬，A/B 比的是字节。
	want := "# 生产环境\n" +
		"ip,port,user,password,key,key_password\n" +
		"10.0.0.1,22,root,hu****r2,/k/id_rsa,se****ss\n" +
		"10.0.0.2,22,root,****,,\n\n"
	if string(data) != want {
		t.Fatalf("副本不对：\n got=%q\nwant=%q", data, want)
	}
	if strings.Contains(string(data), "hunter2") || strings.Contains(string(data), "sekretpass") {
		t.Fatalf("明文密码留在副本里了：%s", data)
	}
}

// 副本 = 原件的 `cp` 加遮罩：除了凭据那两段，BOM、引号、空白一个字节都不许动。
func TestWriteManifestCopyKeepsEveryOtherByte(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nodes.csv")
	// 第一行带 BOM（记事本、Excel 另存的 CSV 就带），第二行的用户名整段带引号且引号里有逗号。
	body := "\ufeff10.0.0.1,22,root,hunter2,/k/id_rsa,sekretpass\n" +
		"10.0.0.2,22,\"root,ops\",pw12345,,pass99\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatalf("造清单失败：%v", err)
	}
	assets := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatalf("建 assets 失败：%v", err)
	}
	if err := WriteManifestCopy(src, assets); err != nil {
		t.Fatalf("写副本失败：%v", err)
	}

	data, err := os.ReadFile(filepath.Join(assets, "nodes.csv"))
	if err != nil {
		t.Fatalf("副本没落下：%v", err)
	}
	want := "\ufeff10.0.0.1,22,root,hu****r2,/k/id_rsa,se****ss\n" +
		"10.0.0.2,22,\"root,ops\",pw****45,,pa****99\n" +
		"\n"
	if string(data) != want {
		t.Fatalf("副本该等于原件、只换掉凭据那两段：\n got=%q\nwant=%q", data, want)
	}
}

// 内联记录没有文件可备份；路径空着、指向目录、指向不存在的文件，都安静跳过。
func TestWriteManifestCopySkipsNonFiles(t *testing.T) {
	dir := t.TempDir()
	assets := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatalf("建 assets 失败：%v", err)
	}
	cases := []struct{ name, src string }{
		{"内联记录", "10.0.0.1,22,root,pw"},
		{"空路径", ""},
		{"目录", dir},
		{"不存在", filepath.Join(dir, "nope.csv")},
	}
	for _, c := range cases {
		if err := WriteManifestCopy(c.src, assets); err != nil {
			t.Fatalf("%s 不该报错：%v", c.name, err)
		}
	}
	entries, err := os.ReadDir(assets)
	if err != nil {
		t.Fatalf("读 assets 失败：%v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("这几种情况都不该产文件，实际 %d 个", len(entries))
	}
}
