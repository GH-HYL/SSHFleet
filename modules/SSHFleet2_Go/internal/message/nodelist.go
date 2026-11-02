package message

import "sshfleet2/internal/contract"

// 读清单那一段要说的话：一句提示、两处失败文案、一条去路。
//
// 清单是使用者天天手改的文件，这几句是最先撞上的那一屏，逐字照老版。

// ansiCyan 是 [INFO] 那种提示头的颜色（老版 common/color.go 的 36）。
const ansiCyan = "\x1b[36m"

// NoNodeParsed 是「一行都没读出来」那句话。老版在三个位置都用它，一字不差。
const NoNodeParsed = "CSV 中未解析出任何有效节点，请检查节点文件或内联文本内容"

// ManifestGoneHint 是「`-f` 那格既不是文件、也不像一条记录」时的去路。
const ManifestGoneHint = "要临时传一台机器，可以直接写一行节点信息，第一个字段是 IP，例如 192.168.1.1,22,root,密码"

// ManifestHeaderDropped 是「第一行不是 IP，已当表头移除」那一行提示。
//
// 老版形态：青色 [INFO] + 黄色 [function:read_nodes_infos] + 正文，后跟一个换行。
// 解站自己不写终端，这一屏挂在纸上，由塔尖打出去（Plan.Notices）。
func ManifestHeaderDropped() contract.Text {
	s := ansiCyan + "[INFO]" + ansiReset +
		ansiYellow + " [function:read_nodes_infos]" + ansiReset +
		" 第一行不是IP格式，已移除表头行\n"
	return contract.Text{S: s}
}
