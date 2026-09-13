package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// lockedBuffer 带锁缓冲：stdout 与 stderr 合并写入、保持顺序（旧 threadSafeWriter）。
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *lockedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *lockedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// RunCommand 连接并执行一条命令/脚本（stdin 为空表示不喂输入）。
// 超时与中断：执行超时经 context.WithTimeout 表达（spec M3 差异，
// 旧实现是 select + time.After 建一个不会被取消的 timer）。
func (c *Client) RunCommand(ctx context.Context, command, stdin string, seq int) *Result {
	result := &Result{
		Seq:  seq,
		IP:   c.cfg.IP,
		Port: c.cfg.Port,
		User: c.cfg.User,
	}

	start := time.Now()
	if err := c.Connect(ctx); err != nil {
		result.ConnectCostTime = time.Since(start).Seconds()
		result.ConnectSuccess = false
		result.Error = strPtr(err.Error())
		return result
	}
	defer func() { _ = c.Close() }()
	result.ConnectCostTime = time.Since(start).Seconds()
	result.ConnectSuccess = true

	session, err := c.conn.NewSession()
	if err != nil {
		result.Error = strPtr("创建会话失败 - " + err.Error())
		return result
	}
	defer func() { _ = session.Close() }()

	out := &lockedBuffer{}
	session.Stdout = out
	session.Stderr = out

	if stdin != "" {
		w, werr := session.StdinPipe()
		if werr != nil {
			result.Error = strPtr("创建输入通道失败 - " + werr.Error())
			return result
		}
		go func() {
			_, _ = io.WriteString(w, stdin)
			_ = w.Close()
		}()
	}

	execStart := time.Now()
	err = c.runWithTimeoutAndCancel(ctx, session, command)
	result.ExecCostTime = time.Since(execStart).Seconds()
	result.Output = out.String()

	if err != nil {
		if code := extractExitCode(err); code != nil {
			result.ExitCode = code
		} else {
			result.Error = strPtr(err.Error())
		}
	} else {
		code := 0
		result.ExitCode = &code
	}
	return result
}

// runWithTimeoutAndCancel 执行超时（context.WithTimeout + defer cancel）与外部中断。
func (c *Client) runWithTimeoutAndCancel(parent context.Context, session *ssh.Session, command string) error {
	ctx, cancel := context.WithTimeout(parent, c.cfg.ExecTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = session.Close()
		if parent.Err() != nil {
			return parent.Err() // 外部取消（中断 / 上级超时）
		}
		return fmt.Errorf("命令执行超时(%v)", c.cfg.ExecTimeout)
	}
}

// extractExitCode 仅 *ssh.ExitError（命令以非 0 退出）返回真实退出码；
// 超时 / 中断等非命令执行错误返回 nil（CONTEXT「退出码」）。
func extractExitCode(err error) *int {
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitStatus()
		return &code
	}
	return nil
}

func strPtr(s string) *string { return &s }

func intPtr(i int) *int { return &i }
