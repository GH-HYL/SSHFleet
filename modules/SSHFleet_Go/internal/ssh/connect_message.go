package ssh

// 建连与认证一摊：把 Go / 系统的原始错误**无损翻译**成报文（ADR-0021 决定第 8 条）。
//
// 这里只做翻译，不下结论：环节名说的是「哪一步、什么现象」（refused / handshake_timeout /
// auth_failed），不是「这算哪一类」。叫它「拒绝网络连接」还是「端口没开」是配置文件的事。
// 冒号后面仍然是原始错误的原样文本——那是权威原文，含混信息一律不吞（决定第 6 条）。

import (
	"context"
	"errors"
	"strings"
)

// 建连一摊的环节名。判定层与配置只认这几个字符串，改名等于让那一类失败落到「未归类」。
const (
	connectRefused          = "refused"
	connectTimeout          = "timeout"
	connectNoRoute          = "no_route"
	connectDNSFailed        = "dns_failed"
	connectDialFailed       = "dial_failed"
	connectHandshakeTimeout = "handshake_timeout"
	connectAuthFailed       = "auth_failed"
	connectHandshakeFailed  = "handshake_failed"
	connectKeyParseFailed   = "key_parse_failed"
	connectKeyNoPassphrase  = "key_passphrase_missing"
)

// connectStageOfDialError 拨号失败（TCP 都还没连上）的环节名。
//
// 取消不给名字：调用方按 ctx 判出 Canceled 后走 abort 那一摊，
// 这里把它归到通用的 dial_failed，免得被读成「端口没开」。
func connectStageOfDialError(err error) string {
	if err == nil || errors.Is(err, context.Canceled) {
		return connectDialFailed
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "actively refused"),
		strings.Contains(msg, "connection reset"):
		return connectRefused
	case strings.Contains(msg, "no route to host"),
		strings.Contains(msg, "network is unreachable"),
		strings.Contains(msg, "host is unreachable"):
		return connectNoRoute
	case strings.Contains(msg, "no such host"):
		return connectDNSFailed
	case strings.Contains(msg, "i/o timeout"), strings.Contains(msg, "timeout"):
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
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "no supported methods remain") {
		return connectAuthFailed
	}
	return connectHandshakeFailed
}

// connectMessage 把一个已定好环节名的错误写成报文。
func connectMessage(stage string, err error) string {
	if err == nil {
		return Message("connect", stage, "")
	}
	return Message("connect", stage, err.Error())
}
