package nodelist

import (
	"fmt"
	"strconv"
	"strings"

	"sshfleet/internal/cli"
	"sshfleet/internal/common"
	"sshfleet/internal/config"
)

// FieldMemory 跨节点累积的输入记忆：是否把本次交互输入应用到后续空字段节点。
// 端口 / 用户名 / 密码各持一组独立记忆（CONTEXT.md「输入记忆」）。
type FieldMemory struct {
	portUseInput       bool
	portInputValue     int
	userUseInput       bool
	userInputValue     string
	passwordUseInput   bool
	passwordInputValue string
}

// resolveNodes 逐节点解析（字段补全三套函数不合并，spec 明确不动）。
func resolveNodes(rows [][]string, pre *precheckResult, args *cli.Args, cfg *config.Config, in *common.Interactor) ([]NodeInfo, error) {
	keyMode := args.KeyMode()
	mem := &FieldMemory{}
	var (
		nodes []NodeInfo
		errs  []string
	)
	total := len(rows)
	for idx, raw := range rows {
		row := padRow(raw)
		rc := pre.rows[idx]
		node, rowErrs := parseNode(row, idx+1, total, keyMode, pre, cfg, args, mem, in)
		if len(rowErrs) > 0 {
			ip := strings.TrimSpace(row[0])
			if ip == "" {
				ip = "空值"
			}
			errs = append(errs, fmt.Sprintf("行 %d (IP: %s): - %s", idx+1, ip, strings.Join(rowErrs, "，")))
			continue
		}
		_ = rc
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

// parseNode 解析单个节点行：IP 校验（D13）+ 字段补全 + 密钥内容 / 口令归属。
func parseNode(row []string, idx, total int, keyMode cli.KeyMode, pre *precheckResult, cfg *config.Config, args *cli.Args, mem *FieldMemory, in *common.Interactor) (NodeInfo, []string) {
	ip := strings.TrimSpace(row[0])
	var errs []string

	// IP：必须存在 + 严格 IPv4（D13，旧版正则不校验每段范围的缺陷在此修正）
	if ip == "" {
		errs = append(errs, "IP必须存在")
	} else if !isStrictIPv4(ip) {
		errs = append(errs, "IP格式不正确")
	}

	port, portErrs := resolvePort(strings.TrimSpace(row[1]), cfg.Account.Port, mem, idx, total, ip, args.Disinteractive, in)
	errs = append(errs, portErrs...)
	user := resolveUser(strings.TrimSpace(row[2]), cfg.Account.User, mem, idx, total, ip, args.Disinteractive, in)
	password := resolvePassword(pre, cfg, mem, idx, total, ip, args.Disinteractive, in)

	// 密钥内容：状态3 用统一私钥原文；其余为预检时读好的 PEM（本行有路径才有值）
	keyContent := ""
	if keyMode == cli.KeyModeUniversal {
		keyContent = pre.universalKeyContent
	} else if pre.rows[idx-1].keyContent != "" {
		keyContent = pre.rows[idx-1].keyContent
	}

	// 私钥口令：状态3 用统一口令；状态1/2 CSV 第6列优先，缺省用全局配置
	keyPassphrase := ""
	if keyMode == cli.KeyModeUniversal {
		keyPassphrase = pre.universalPassphrase
	} else if pre.rows[idx-1].keyPassRaw != "" {
		keyPassphrase = pre.rows[idx-1].keyPassRaw
	} else if pre.globalPassphrase != "" {
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
		KeyContent:    keyContent,
		KeyPassphrase: keyPassphrase,
	}, nil
}

// resolvePort 端口字段补全：CSV > config > 输入记忆 > 交互输入。
func resolvePort(raw string, defaultPort int, mem *FieldMemory, idx, total int, ip string, disinteractive bool, in *common.Interactor) (int, []string) {
	if raw != "" {
		// 对位旧 `port.isdigit()`：只接受纯数字（不接受正负号）
		v, err := strconv.Atoi(raw)
		if err != nil || !isAllDigits(raw) || v < 1 || v > 65535 {
			return 0, []string{"端口必须是1-65535之间的整数，当前值为：" + raw}
		}
		return v, nil
	}
	if defaultPort != 0 {
		if defaultPort < 1 || defaultPort > 65535 {
			return 0, []string{"配置文件中的默认端口格式错误"}
		}
		return defaultPort, nil
	}
	if mem.portUseInput {
		return mem.portInputValue, nil
	}
	val, err := in.Prompt(fmt.Sprintf("行 %d (IP: %s): 端口为空，请输入端口号: ", idx, ip))
	if err != nil {
		return 0, []string{err.Error()}
	}
	for {
		v, cerr := strconv.Atoi(val)
		if cerr == nil && v >= 1 && v <= 65535 {
			// 询问是否将此端口号应用于所有后续端口为空的节点
			if !mem.portUseInput && idx < total {
				yes, cerr2 := in.Confirm(fmt.Sprintf("\n是否将此端口号应用于所有后续端口为空的节点？"), true)
				if cerr2 != nil {
					return 0, []string{cerr2.Error()}
				}
				if yes {
					mem.portUseInput = true
					mem.portInputValue = v
				}
			}
			return v, nil
		}
		fmt.Printf("端口必须是1-65535之间的整数，当前输入：%s\n", val)
		val, err = in.Prompt(fmt.Sprintf("行 %d (IP: %s): 端口为空，请输入端口号: ", idx, ip))
		if err != nil {
			return 0, []string{err.Error()}
		}
	}
}

// isAllDigits 纯数字判定（对位旧 str.isdigit()）。
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

// resolveUser 用户名字段补全：CSV > config > 输入记忆 > 交互输入。
func resolveUser(raw, defaultUser string, mem *FieldMemory, idx, total int, ip string, disinteractive bool, in *common.Interactor) string {
	if raw != "" {
		return raw
	}
	if defaultUser != "" {
		return defaultUser
	}
	if mem.userUseInput {
		return mem.userInputValue
	}
	val, err := in.Prompt(fmt.Sprintf("行 %d (IP: %s): 用户名为空，请输入用户名: ", idx, ip))
	if err != nil {
		return ""
	}
	for strings.TrimSpace(val) == "" {
		fmt.Println("用户名不能为空")
		val, err = in.Prompt(fmt.Sprintf("行 %d (IP: %s): 用户名为空，请输入用户名: ", idx, ip))
		if err != nil {
			return ""
		}
	}
	// 询问是否将此用户名应用于所有后续用户为空的节点
	if !mem.userUseInput && idx < total {
		yes, cerr := in.Confirm(fmt.Sprintf("\n是否将此用户名应用于所有后续用户为空的节点？"), true)
		if cerr == nil && yes {
			mem.userUseInput = true
			mem.userInputValue = val
		}
	}
	return val
}

// resolvePassword 密码字段补全：CSV > config > 密钥认证 > 输入记忆 > 交互输入。
// 解码值取自预检结果（读→校验→直接用，不再读盘）。
func resolvePassword(pre *precheckResult, cfg *config.Config, mem *FieldMemory, idx, total int, ip string, disinteractive bool, in *common.Interactor) string {
	if pre.rows[idx-1].passwordPlain != "" {
		return pre.rows[idx-1].passwordPlain
	}
	if pre.defaultPasswordPlain != "" {
		return pre.defaultPasswordPlain
	}
	if pre.rows[idx-1].hasKey {
		return ""
	}
	if mem.passwordUseInput {
		return mem.passwordInputValue
	}
	val, err := in.PromptPassword(fmt.Sprintf("行 %d (IP: %s): 密码为空，请输入密码: ", idx, ip))
	if err != nil {
		return ""
	}
	for val == "" {
		fmt.Println("密码不能为空，请重新输入")
		val, err = in.PromptPassword(fmt.Sprintf("行 %d (IP: %s): 密码为空，请输入密码: ", idx, ip))
		if err != nil {
			return ""
		}
	}
	// 询问是否将此密码应用于所有后续密码为空的节点
	if !mem.passwordUseInput && idx < total {
		yes, cerr := in.Confirm(fmt.Sprintf("\n是否将此密码应用于所有后续密码为空的节点？"), true)
		if cerr == nil && yes {
			mem.passwordUseInput = true
			mem.passwordInputValue = val
		}
	}
	return val
}
