package archive

// 备份执行资源：把这次用到的脚本与代填文件原样复制进本次目录的 `assets/`。
//
// 只复制、不改写：认列、遮秘密是 `mask` 的事（清单的脱敏副本归它）。本包在这里只做一件
// 整件事——「把这几个文件放进 assets」，不认内容、不懂格式。

import (
	"fmt"
	"os"
	"path/filepath"
)

// Backup 把 sources 里点名的文件原样复制进 assetsDir，文件名不变。
//
// 跳过的三种情况照老版 `copyFile`：空串（这次没用这条）、目录、找不到——都不是错误，
// 使用者给了个目录或路径已被删掉，备份这一步不该把整次运行拖垮。
// 同名文件覆盖（本次目录是新建的，正常情况下不会撞）。
func Backup(assetsDir string, sources ...string) error {
	for _, src := range sources {
		if src == "" {
			continue
		}
		info, err := os.Stat(src)
		if err != nil || info.IsDir() {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("读不到要备份的文件 %s：%w", src, err)
		}
		dst := filepath.Join(assetsDir, filepath.Base(src))
		if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
			return fmt.Errorf("写不进 %s：%w", dst, err)
		}
	}
	return nil
}
