// 错误分类（对位旧 classifier.py，ADR-0003 退出码语义）：
//
//	exit_code = 0    → 命令全部成功（传输模式为「传输成功」）
//	exit_code ≠ 0    → 有命令失败，退出码是权威信号，不再匹配关键词
//	exit_code = nil  → 未执行任何命令（连接失败 / 传输失败 / 超时 / 中断），靠关键词推断
//
// 关键词未命中时把报错原文本身作为分类（保留具体失败内容），仅 error 与 output 均空时才兜底。
package result

import "strings"

const (
	// SuccessCategoryExecute 命令模式的成功分类。
	SuccessCategoryExecute = "执行成功"
	// SuccessCategoryTransport 传输模式（上传/下载）的成功分类。
	SuccessCategoryTransport = "传输成功"
	// PartialSuccessCategory 传输部分成功（有成功有失败）。
	PartialSuccessCategory = "部分成功"
	// Unclassified 完全无信息时的兜底分类。
	Unclassified = "错误未分类"
	// fallbackMaxLen 兜底原文的最大长度（超出截断，避免长报文撑爆统计/报表）。
	fallbackMaxLen = 200
)

// Case 一次分类的输入。
type Case struct {
	ExitCode     *int
	Error        string
	Output       string
	AuthFailure  string // 认证失败分类（spec D44）：私钥与密码都尝试且都失败时由 ssh 层给出
	Mode         string // execute / upload / download
	SuccessFiles int
	FailedFiles  int
}

// Classify 按响应字段给出一条分类名。
func Classify(c Case, kw *Keywords) string {
	// 有退出码：按退出码分类，不再对输出做关键词匹配
	if c.ExitCode != nil {
		if *c.ExitCode == 0 {
			if isTransport(c.Mode) {
				return SuccessCategoryTransport
			}
			return SuccessCategoryExecute
		}
		return "执行失败(退出码" + itoa(*c.ExitCode) + ")"
	}

	// 认证失败分类（D44）：显式字段优先于关键词推断
	if c.AuthFailure != "" {
		return c.AuthFailure
	}

	// 传输模式部分成功优先：有成功有失败是结构性状态，先于失败原因展示
	if isTransport(c.Mode) && c.FailedFiles > 0 && c.SuccessFiles > 0 {
		return PartialSuccessCategory
	}

	if c.Error != "" {
		if hit := kw.match(c.Error); hit != "" {
			return hit
		}
		if trimmed := strings.TrimSpace(c.Error); trimmed != "" {
			return truncate(trimmed)
		}
		return Unclassified
	}

	if c.Output != "" {
		if hit := kw.match(c.Output); hit != "" {
			return hit
		}
		if trimmed := strings.TrimSpace(c.Output); trimmed != "" {
			return truncate(trimmed)
		}
		return Unclassified
	}

	return Unclassified
}

// IsFallbackCategory 判断分类名是否为「关键词未命中时的兜底原文」。
func IsFallbackCategory(category string, kw *Keywords) bool {
	if category == "" || category == Unclassified {
		return false
	}
	if kw.Has(category) {
		return false
	}
	if category == SuccessCategoryExecute || category == SuccessCategoryTransport || category == PartialSuccessCategory {
		return false
	}
	if strings.HasPrefix(category, "执行失败(退出码") {
		return false
	}
	return true
}

// IsSuccessResult 结果是否算成功（连接成功且退出码为 0）。
func IsSuccessResult(exitCode *int, connectSuccess bool) bool {
	return connectSuccess && exitCode != nil && *exitCode == 0
}

func isTransport(mode string) bool { return mode == "upload" || mode == "download" }

func truncate(text string) string {
	if len(text) <= fallbackMaxLen {
		return text
	}
	// 按 rune 截断：按字节切会把多字节字符切成无效 UTF-8（2026-09-14 审计修复）
	r := []rune(text)
	out := make([]rune, 0, fallbackMaxLen)
	count := 0
	for _, c := range r {
		if count >= fallbackMaxLen {
			break
		}
		out = append(out, c)
		count++
	}
	return string(out) + "…"
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
