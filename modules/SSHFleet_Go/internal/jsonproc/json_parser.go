package jsonproc

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 业务错误码（第一层 HTTP 请求级错误）
const (
	CodeInvalidRequest = "INVALID_REQUEST" // JSON 语法错误
	CodeMissingField   = "MISSING_FIELD"   // 必填字段缺失或无效
)

// APIError 请求解析/校验错误，携带业务错误码
// 供 handler 层通过 errors.As 提取错误码，写入 HTTP 错误响应
type APIError struct {
	Code    string // 业务错误码
	Message string // 位置描述 + 原文
}

// Error 实现 error 接口
func (e *APIError) Error() string { return e.Message }

// NewAPIError 构造带错误码的校验错误
func NewAPIError(code, message string) *APIError {
	return &APIError{Code: code, Message: message}
}

// parseJSON 解析 JSON，语法错误 → INVALID_REQUEST（决策 A）
func parseJSON(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return NewAPIError(CodeInvalidRequest, fmt.Sprintf("JSON 解析失败: %v", err))
	}
	return nil
}

// validateNodes 节点级必填校验 + port 校验（决策 B / B5）
// ip 空 / user 空 / password 与 key_content 同时空 → MISSING_FIELD（带节点序号）
// port: 0=缺省（JSON 不传即 0，默认 22）；非 0 必须 1~65535
func validateNodes(nodes []NodeInfo) error {
	for i, node := range nodes {
		pos := fmt.Sprintf("nodes[%d] 校验失败", i)
		if strings.TrimSpace(node.IP) == "" {
			return NewAPIError(CodeMissingField, fmt.Sprintf("%s: ip 不能为空", pos))
		}
		if strings.TrimSpace(node.User) == "" {
			return NewAPIError(CodeMissingField, fmt.Sprintf("%s: user 不能为空", pos))
		}
		if strings.TrimSpace(node.Password) == "" && strings.TrimSpace(node.KeyContent) == "" {
			return NewAPIError(CodeMissingField, fmt.Sprintf("%s: password 与 key_content 不能同时为空", pos))
		}
		if node.Port != 0 && (node.Port < 1 || node.Port > 65535) {
			return NewAPIError(CodeMissingField, fmt.Sprintf("%s: port %d 超出范围 1~65535", pos, node.Port))
		}
	}
	return nil
}

// ParseRequest 解析 HTTP 请求体字节
func ParseRequest(data []byte) (*ExecuteRequest, error) {
	var req ExecuteRequest
	if err := parseJSON(data, &req); err != nil {
		return nil, err
	}

	// 输入验证（决策 E1：纯空白也判空）
	if strings.TrimSpace(req.Command) == "" {
		return nil, NewAPIError(CodeMissingField, "command 不能为空")
	}
	if len(req.Nodes) == 0 {
		return nil, NewAPIError(CodeMissingField, "nodes 数组不能为空")
	}

	// seq 查重
	seen := make(map[int]bool, len(req.Nodes))
	for _, node := range req.Nodes {
		if seen[node.Seq] {
			return nil, NewAPIError(CodeMissingField, fmt.Sprintf("seq %d 重复", node.Seq))
		}
		seen[node.Seq] = true
	}

	// 节点级必填 + port 校验
	if err := validateNodes(req.Nodes); err != nil {
		return nil, err
	}

	// 默认值填充
	if req.Options.Concurrency <= 0 || req.Options.Concurrency > len(req.Nodes) {
		req.Options.Concurrency = len(req.Nodes)
	}
	if req.Options.ConnectTimeout <= 0 {
		req.Options.ConnectTimeout = 10
	}
	if req.Options.ExecTimeout <= 0 {
		return nil, NewAPIError(CodeMissingField, "exec_timeout 不能为空且必须大于 0")
	}
	for i := range req.Nodes {
		if req.Nodes[i].Port <= 0 {
			req.Nodes[i].Port = 22
		}
	}

	return &req, nil
}

// ParseUploadRequest 解析上传请求体字节
func ParseUploadRequest(data []byte) (*UploadRequest, error) {
	var req UploadRequest
	if err := parseJSON(data, &req); err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.FilePath) == "" {
		return nil, NewAPIError(CodeMissingField, "file_path 不能为空")
	}
	if strings.TrimSpace(req.RemotePath) == "" {
		return nil, NewAPIError(CodeMissingField, "remote_path 不能为空")
	}
	if len(req.Nodes) == 0 {
		return nil, NewAPIError(CodeMissingField, "nodes 数组不能为空")
	}
	if req.Options.ExecTimeout <= 0 {
		return nil, NewAPIError(CodeMissingField, "exec_timeout 必须大于 0")
	}

	// seq 查重
	seen := make(map[int]bool, len(req.Nodes))
	for _, node := range req.Nodes {
		if seen[node.Seq] {
			return nil, NewAPIError(CodeMissingField, fmt.Sprintf("seq %d 重复", node.Seq))
		}
		seen[node.Seq] = true
	}

	// 节点级必填 + port 校验
	if err := validateNodes(req.Nodes); err != nil {
		return nil, err
	}

	// 默认值填充
	if req.Options.Concurrency <= 0 || req.Options.Concurrency > len(req.Nodes) {
		req.Options.Concurrency = len(req.Nodes)
	}
	if req.Options.ConnectTimeout <= 0 {
		req.Options.ConnectTimeout = 10
	}
	for i := range req.Nodes {
		if req.Nodes[i].Port <= 0 {
			req.Nodes[i].Port = 22
		}
	}

	return &req, nil
}

// ParseDownloadRequest 解析下载请求体字节
func ParseDownloadRequest(data []byte) (*DownloadRequest, error) {
	var req DownloadRequest
	if err := parseJSON(data, &req); err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.RemotePath) == "" {
		return nil, NewAPIError(CodeMissingField, "remote_path 不能为空")
	}
	if strings.TrimSpace(req.LocalPath) == "" {
		return nil, NewAPIError(CodeMissingField, "local_path 不能为空")
	}
	if len(req.Nodes) == 0 {
		return nil, NewAPIError(CodeMissingField, "nodes 数组不能为空")
	}
	if req.Options.ExecTimeout <= 0 {
		return nil, NewAPIError(CodeMissingField, "exec_timeout 必须大于 0")
	}

	// seq 查重
	seen := make(map[int]bool, len(req.Nodes))
	for _, node := range req.Nodes {
		if seen[node.Seq] {
			return nil, NewAPIError(CodeMissingField, fmt.Sprintf("seq %d 重复", node.Seq))
		}
		seen[node.Seq] = true
	}

	// 节点级必填 + port 校验
	if err := validateNodes(req.Nodes); err != nil {
		return nil, err
	}

	// 默认值填充
	if req.Options.Concurrency <= 0 || req.Options.Concurrency > len(req.Nodes) {
		req.Options.Concurrency = len(req.Nodes)
	}
	if req.Options.ConnectTimeout <= 0 {
		req.Options.ConnectTimeout = 10
	}
	for i := range req.Nodes {
		if req.Nodes[i].Port <= 0 {
			req.Nodes[i].Port = 22
		}
	}

	return &req, nil
}
