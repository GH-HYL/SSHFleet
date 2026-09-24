package nodelist

import (
	"fmt"
	"strconv"
	"strings"

	"sshfleet/internal/config"
)

// 输出配色：提示黄、[INFO] 青配黄 function。
const (
	colorReset  = "\x1b[0m"
	colorCyan   = "\x1b[36m"
	colorYellow = "\x1b[33m"
)

// resolveNodes 逐节点解析：字段补全（清单 > 配置，缺了就报错，不再询问）。
func resolveNodes(rows [][]string, pre *precheckResult, cfg *config.Config) ([]NodeInfo, error) {
	var (
		nodes []NodeInfo
		errs  []string
	)
	for idx, raw := range rows {
		row := padRow(raw)
		// 行号对外一律 1 基（提示文案「行 N」与 parseNode 同口径）；凭据经 rowCreds 取用
		node, rowErrs := parseNode(row, idx+1, pre, cfg)
		if len(rowErrs) > 0 {
			ip := strings.TrimSpace(row[0])
			if ip == "" {
				ip = "空值"
			}
			errs = append(errs, fmt.Sprintf("行 %d (IP: %s): - %s", idx+1, ip, strings.Join(rowErrs, "，")))
			continue
		}
		nodes = append(nodes, node)
	}
	if len(errs) > 0 {
		var b strings.Builder
		b.WriteString("发现以下错误:\n")
		for _, e := range errs {
			fmt.Fprintf(&b, "%s\n", e)
		}
		return nil, fmt.Errorf("%s", strings.TrimSuffix(b.String(), "\n"))
	}
	return nodes, nil
}

// parseNode 解析单个节点行：IP 校验 + 字段补全 + 密钥内容 / 口令归属。
// idx 为**1 基行号**（提示文案与凭据取用同一个口径）。
func parseNode(row []string, idx int, pre *precheckResult, cfg *config.Config) (NodeInfo, []string) {
	ip := strings.TrimSpace(row[0])
	var errs []string
	rc := pre.rowCreds(idx)

	// IP：必须存在 + 严格 IPv4（旧版正则不校验每段范围的缺陷在此修正）
	if ip == "" {
		errs = append(errs, "IP必须存在")
	} else if !isStrictIPv4(ip) {
		errs = append(errs, "IP格式不正确")
	}

	port, portErrs := resolvePort(strings.TrimSpace(row[1]), cfg.Account.Port)
	errs = append(errs, portErrs...)
	user, userErr := resolveUser(strings.TrimSpace(row[2]), cfg.Account.User)
	if userErr != nil {
		errs = append(errs, userErr.Error())
	}
	password, passErr := resolvePassword(rc, pre)
	if passErr != nil {
		errs = append(errs, passErr.Error())
	}

	// 私钥口令：清单第 6 列优先，缺省用配置里的
	keyPassphrase := rc.keyPassRaw
	if keyPassphrase == "" {
		keyPassphrase = pre.globalPassphrase
	}

	if len(errs) > 0 {
		return NodeInfo{}, errs
	}
	return NodeInfo{
		IP:            ip,
		Port:          port,
		User:          user,
		Password:      password,
		KeyContent:    rc.keyContent,
		KeyPassphrase: keyPassphrase,
	}, nil
}

// resolvePort 端口补全：清单第 2 列 > 配置 account.port。
func resolvePort(raw string, defaultPort int) (int, []string) {
	if raw != "" {
		// 只接受纯数字（不接受正负号）
		v, err := strconv.Atoi(raw)
		if err != nil || !isAllDigits(raw) || v < 1 || v > 65535 {
			return 0, []string{"端口必须是1-65535之间的整数，当前值为：" + raw}
		}
		return v, nil
	}
	// 配置预检查已保证 account.port ∈ [1,65535]
	return defaultPort, nil
}

// isAllDigits 纯数字判定。
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// resolveUser 用户名补全：清单第 3 列 > 配置 account.user。
func resolveUser(raw, defaultUser string) (string, error) {
	if raw != "" {
		return raw, nil
	}
	if defaultUser != "" {
		return defaultUser, nil
	}
	return "", fmt.Errorf("用户名为空：清单第 3 列没写，配置 account.user 也没配")
}

// resolvePassword 密码补全：清单第 4 列 > 密钥登录（有私钥则不留密码）> 配置 account.password。
// 注：私钥节点密码恒为空，不受其他节点是否使用默认密码影响（避免混合清单里的状态泄漏）。
func resolvePassword(rc rowCreds, pre *precheckResult) (string, error) {
	if rc.passwordPlain != "" {
		return rc.passwordPlain, nil
	}
	if rc.hasKey {
		return "", nil
	}
	if pre.defaultPasswordPlain != "" {
		return pre.defaultPasswordPlain, nil
	}
	return "", fmt.Errorf("密码为空：清单第 4 列没写，配置 account.password 也没配")
}
