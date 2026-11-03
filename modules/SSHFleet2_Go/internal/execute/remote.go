package execute

// 辅助命令：工具在远端额外跑的那些（`test -e`、`find`、`sudo mkdir`、`sudo mv`、`sudo rm`）。
//
// 它们不是「目的命令」：退出码进 `NodeResult.Steps`，不顶成结果退出码——否则判定会给出
// 「执行失败(退出码N)」，把远端说的原因整个盖掉。

import "strings"

// exec 跑一条辅助命令，不采集输出。
func (c *client) exec(command string) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()
	return session.Run(command)
}

// execCapture 跑一条辅助命令并取回合并输出（用于枚举远端文件）。
func (c *client) execCapture(command string) (string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", err
	}
	defer func() { _ = session.Close() }()
	out, err := session.CombinedOutput(command)
	return string(out), err
}

// escapeShellArg 把路径嵌进远端命令的单引号里之前的转义。
//
// 转义规则与解析站拼下发行那一处同源（`'\”`），不再各写一遍——`find`／`mv`／`rm`
// 三处都靠它，写歪一处就会把远端命令拼坏。
func escapeShellArg(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}
