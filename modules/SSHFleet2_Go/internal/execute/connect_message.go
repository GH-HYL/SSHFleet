package execute

// 建连与认证这一摊：把 Go 与系统给的原始错误**无损翻译**成报文（ADR-0021 决定第 8 条）。
//
// 这里只做翻译、不下结论：环节名说的是「哪一步、什么现象」（refused / handshake_timeout /
// auth_failed），不是「这算哪一类」。叫它「拒绝网络连接」还是「端口没开」是判据表里那条
// 分类的事；冒号后面仍然是原始错误的原样文本——那是权威原文，含混信息一律不吞（决定第 6 条）。
//
// 搬自老版 `internal/ssh/connect_message.go`，环节名一个字没改：判定与配置按这几个字符串
// 认人，改名等于让那一类失败落到「未归类」。

import (
	"context"
	"errors"
	"strings"
)

// 建连这一摊的环节名。判定层与判据表只认这几个字符串。
const (
	connectRefused          = "refused"
	connectTimeout          = "timeout"
	connectNoRoute          = "no_route"
	connectDialFailed       = "dial_failed"
	connectHandshakeTimeout = "handshake_timeout"
	connectAuthFailed       = "auth_failed"
	connectHandshakeFailed  = "handshake_failed"
	connectKeyParseFailed   = "key_parse_failed"
	connectKeyNoPassphrase  = "key_passphrase_missing"
)

// connectStageOfDialError 拨号失败（TCP 都还没连上）的环节名。
//
// 取消不给名字：调用方按 ctx 判出「被取消」后自己记那一格事实，
// 这里把它归到通用的 dial_failed，免得被读成「端口没开」。
func connectStageOfDialError(err error) string {
	if err == nil || errors.Is(err, context.Canceled) {
		return connectDialFailed
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "connection refused"),
		strings.Contains(text, "actively refused"),
		strings.Contains(text, "connection reset"):
		return connectRefused
	case strings.Contains(text, "no route to host"),
		strings.Contains(text, "network is unreachable"),
		strings.Contains(text, "host is unreachable"):
		return connectNoRoute
	// 没有 dns_failed：清单只收 IP，不发域名解析请求（老版 2026-10-20 定）。
	// 真出现 "no such host" 也只可能是别处转来的文本，按通用拨号失败报。
	case strings.Contains(text, "i/o timeout"), strings.Contains(text, "timeout"):
		return connectTimeout
	default:
		return connectDialFailed
	}
}

// connectStageOfHandshakeError 握手阶段（TCP 通了、SSH 协议没谈成）的环节名。
func connectStageOfHandshakeError(err error) string {
	if err == nil {
		return connectHandshakeFailed
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "unable to authenticate") ||
		strings.Contains(text, "no supported methods remain") {
		return connectAuthFailed
	}
	return connectHandshakeFailed
}

// connectMessage 把一个已定好环节名的错误写成报文。
func connectMessage(stage string, err error) string {
	if err == nil {
		return msg(envConnect, stage, "")
	}
	return msg(envConnect, stage, err.Error())
}
