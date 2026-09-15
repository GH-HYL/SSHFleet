// Package nodelist 承载主干第 6 步：读取节点清单 + 字段补全 + 输入记忆。
//
// 节点标识只支持 IPv4 字面量，严格校验（spec D13）；CSV 读取必须
// FieldsPerRecord = -1（容忍变长行）；凭据解码缓存的替代形态：预检阶段
// 「读 → 校验 → 直接用」，解码值进入预检结果，逐节点解析不再读盘。
package nodelist

import (
	"net/netip"
	"strings"

	"sshfleet/internal/cli"
	"sshfleet/internal/common"
	"sshfleet/internal/config"
)

// NodeInfo 单台目标主机的连接要素（纯数据载体，CONTEXT.md「节点信息」）。
type NodeInfo struct {
	IP            string
	Port          int
	User          string
	Password      string
	KeyContent    string
	KeyPassphrase string
}

// Nodes 节点信息集合。
type Nodes struct{ Items []NodeInfo }

// Len 节点数量。
func (n *Nodes) Len() int { return len(n.Items) }

// rowCreds 预检产出的单行凭据解码值：逐节点解析阶段直接使用，不再读盘。
type rowCreds struct {
	passwordPlain string // 密码列解码结果（列空则空串）
	keyContent    string // 三态解析后实际使用的私钥 PEM 原文
	keyPassRaw    string // 与该节点私钥**同源**的口令明文（成对绑定，spec D42）
	hasKey        bool   // 解析后是否持有私钥（决定密码段跳过交互）
	keyFromConfig bool   // 私钥来源为配置文件（决定口令来源）
}

// precheckResult 凭据预检的汇总产出。
//
// rows 与清单行**按位置同序对齐**（第 i 项对应第 i 行，0 基）；逐节点解析侧请统一经
// rowCreds(行号) 取用，不要自己写下标——历史上预检写 rows[idx]（0 基）、解析读
// rows[idx-1]（1 基行号），两套口径并存，靠 padRow 各自兜底，改动时极易错位。
type precheckResult struct {
	rows                 []rowCreds
	needDefaultPassword  bool   // 是否存在依赖默认密码的节点
	anyNodeUsesKey       bool   // 是否存在使用密钥认证的节点（default 态）
	anyNodeUsesConfigKey bool   // 是否存在私钥取自配置文件的节点（决定配置口令是否读取）
	defaultPasswordPlain string // needDefaultPassword 时配置默认密码的解码值
	globalPassphrase     string // 配置中的私钥口令解码值（只配给「私钥取自配置」的节点）
	universalKeyContent  string // 状态3：统一私钥 PEM 原文
	universalPassphrase  string // 状态3：统一口令（交互输入）
}

// rowCreds 取指定清单行的预检凭据，行号用**1 基**（与「行 N」提示文案、CSV 阅读习惯一致）。
// 越界返回零值：预检与解析都以同一份 rows 为准，正常流程不会越界；这里只做防御，
// 保证两个调用点不会因下标口径不同而拿到邻行的凭据。
func (p *precheckResult) rowCreds(lineNo int) rowCreds {
	idx := lineNo - 1
	if idx < 0 || idx >= len(p.rows) {
		return rowCreds{}
	}
	return p.rows[idx]
}

// Read 主干第 6 步入口：CSV/内联 → 预检解码 → 字段补全 → 节点集合。
func Read(args *cli.Args, cfg *config.Config, in *common.Interactor) (*Nodes, error) {
	rows, err := readCSVRows(args.CsvFile, args.FIsInline, in)
	if err != nil {
		return nil, err
	}

	pre, err := precheckCredentials(rows, args, cfg, in)
	if err != nil {
		return nil, err
	}

	nodes, err := resolveNodes(rows, pre, args, cfg, in)
	if err != nil {
		return nil, err
	}
	return &Nodes{Items: nodes}, nil
}

// isStrictIPv4 D13：节点标识只支持 IPv4 字面量，严格校验（netip.ParseAddr）。
func isStrictIPv4(s string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(s))
	return err == nil && addr.Is4()
}
