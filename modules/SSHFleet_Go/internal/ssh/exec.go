package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
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

// newResult 单节点结果的共同开头：四条执行路径（命令 / 脚本 / 上传 / 下载）
// 都从这里起手，避免各写一份 Seq / IP / Port / User 的填充。
func (c *Client) newResult(seq int) *Result {
	return &Result{Seq: seq, IP: c.cfg.IP, Port: c.cfg.Port, User: c.cfg.User}
}

// connectFor 建连并落定结果里的连接字段（成功与否、耗时、错误原文、认证失败分类）。
// 失败时返回 false——调用方据此直接返回该结果，不必各写一遍收尾。
// 成功后连接已建立，调用方负责 defer Close。
func (c *Client) connectFor(ctx context.Context, result *Result) bool {
	start := time.Now()
	if err := c.Connect(ctx); err != nil {
		result.ConnectCostTime = time.Since(start).Seconds()
		result.ConnectSuccess = false
		result.Error = strPtr(err.Error())
		result.AuthFailure = c.classifyAuthFailure(err)
		return false
	}
	result.ConnectCostTime = time.Since(start).Seconds()
	result.ConnectSuccess = true
	return true
}

// RunCommand 连接并执行一条命令/脚本（stdin 为空表示不喂输入）。
// 超时与中断：执行超时经 context.WithTimeout 表达（spec M3 差异，
// 旧实现是 select + time.After 建一个不会被取消的 timer）。
func (c *Client) RunCommand(ctx context.Context, command, stdin string, seq int) *Result {
	result := c.newResult(seq)
	defer c.applyBanner(result) // 服务端提示并入报错原文（ADR-0005）

	if !c.connectFor(ctx, result) {
		return result
	}
	defer func() { _ = c.Close() }()

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
	result.Output = trimOuterBlankLines(out.String())

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

// trimOuterBlankLines 远端输出写进 Output 字段前的整备：只去掉**最前与最后**的空白行，
// 中间原样——中间的空行、每行的行首缩进，都是命令输出自身的结构。
//
// 为什么只在这里做一次：Output 是原始字段，采集侧给出来就是干净的。终端与 output.txt
// 拿它直接输出、不做任何处理；xlsx 那边只额外清非法字符（见 output.cleanForExcel）。
// 整备若散到各输出端，就会各清各的、行为互相漂移。
//
// 为什么不能 TrimSpace：那等于把「块首」当成「行首」。`who -b` 只有一行，它的行首缩进
// 正好落在块首，会被当作首部空白一起吃掉——而那是输出自身的排版（`system boot` 前
// 那串空格是对齐用的）。多行输出只有第一行受害，症状看起来像「随机丢缩进」。
//
// 为什么不能逐行丢弃空白行：空行同样可能是输出的组成部分（`ls -l` 分组、`df -h`、
// 日志分段都靠空行分隔），按行清理会改掉原输出的结构。
func trimOuterBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
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
