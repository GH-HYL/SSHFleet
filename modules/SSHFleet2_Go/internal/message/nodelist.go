package message

// 读清单那一段要说的话：一句提示、两处失败文案、一条去路。
//
// 清单是使用者天天手改的文件，这几句是最先撞上的那一屏，逐字照老版——只有 `-f` 去路
// 那一句按审计改了：原来只说怎么写内联，不说路径可能写错。

// ansiCyan 是 [INFO] 那种提示头的颜色（老版 common/color.go 的 36）。
const ansiCyan = "\x1b[36m"

// NoNodeParsed 是「一行都没读出来」那句话。老版在三个位置都用它，一字不差。
const NoNodeParsed = "CSV 中未解析出任何有效节点，请检查节点文件或内联文本内容"

// InlineNodeExample 是「一条内联记录长什么样」的那个样例。
//
// 一句样例只许有一个出处：`-f` 的去路提示与「没给 -f」那句都要举例，各拼一遍就会
// 各教一种写法。
const InlineNodeExample = "192.168.1.1,22,root,密码"

// ManifestGoneHint 是「`-f` 那格既不是文件、也不像一条记录」时的去路。
//
// 两种给法各指一次：路径对不上就查路径，写的是内联记录就查第一列。
const ManifestGoneHint = "请检查路径是否写对；如果是内联写法，第一列要是合法 IP，例如 " + InlineNodeExample

// CredentialRelativeWithoutSecretDir 是凭据那一格写的是相对名、配置里却没配 secret_dir 时那句。
//
// 这一处不是「这一行的写法错了」，是配置缺了一项：所以句子只说找谁配、或改写成绝对路径，
// 不催使用者去改清单。
func CredentialRelativeWithoutSecretDir() string {
	return "写的是相对路径，但配置里没配 credential.secret_dir（提示：在配置的 [credential] 段配上 secret_dir，或这一格改写本机绝对路径）"
}

// ManifestHeaderDropped 是「第一行不是 IP，已当表头移除」那一行提示。
//
// 老版形态：青色 [INFO] + 黄色 [function:read_nodes_infos] + 正文，后跟一个换行。
// 解站自己不写终端，这一屏挂在纸上，由塔尖打出去（Plan.Notices）。
func ManifestHeaderDropped() Line {
	s := ansiCyan + "[INFO]" + ansiReset +
		ansiYellow + " [function:read_nodes_infos]" + ansiReset +
		" 第一行不是IP格式，已移除表头行\n"
	return Line{s: s}
}
