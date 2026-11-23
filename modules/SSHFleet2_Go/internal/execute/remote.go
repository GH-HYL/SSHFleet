package execute

// 辅助命令：工具在远端额外跑的那些（`test -e`、`find`、`sudo mkdir`、`sudo mv`、`sudo rm`）。
//
// 它们不是「目的命令」：退出码进 `NodeResult.Steps`，不顶成结果退出码——否则判定会给出
// 「执行失败(退出码N)」，把远端说的原因整个盖掉。

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

// execArmed 跑一条辅助命令，按传输预算武装：到点就关掉这条会话、等它退出来。
func (c *client) execArmed(clock *transferClock, command string) (error, armEnd) {
	session, err := c.conn.NewSession()
	if err != nil {
		return err, armRan
	}
	defer func() { _ = session.Close() }()

	var runErr error
	end := clock.arm(func() { _ = session.Close() }, func() { runErr = session.Run(command) })
	return runErr, end
}

// captureArmed 同一条，但只取 stdout：枚举记录按 NUL 切，远端的一句告警混进 stdout
// 会被当成一条坏记录。
func (c *client) captureArmed(clock *transferClock, command string) (string, armEnd, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", armRan, err
	}
	defer func() { _ = session.Close() }()

	var (
		out    []byte
		runErr error
	)
	end := clock.arm(func() { _ = session.Close() }, func() { out, runErr = session.Output(command) })
	return string(out), end, runErr
}
