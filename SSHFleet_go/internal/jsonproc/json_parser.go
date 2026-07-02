package jsonproc

import (
	"encoding/json"
	"fmt"
)

// ParseRequest 解析 HTTP 请求体字节
func ParseRequest(data []byte) (*ExecuteRequest, error) {
	var req ExecuteRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}

	// 输入验证
	if req.Command == "" {
		return nil, fmt.Errorf("command 不能为空")
	}
	if len(req.Nodes) == 0 {
		return nil, fmt.Errorf("nodes 数组不能为空")
	}

	// seq 查重
	seen := make(map[int]bool, len(req.Nodes))
	for _, node := range req.Nodes {
		if seen[node.Seq] {
			return nil, fmt.Errorf("seq %d 重复", node.Seq)
		}
		seen[node.Seq] = true
	}

	// 默认值填充
	if req.Options.Concurrency <= 0 || req.Options.Concurrency > len(req.Nodes) {
		req.Options.Concurrency = len(req.Nodes)
	}
	if req.Options.ConnectTimeout <= 0 {
		req.Options.ConnectTimeout = 10
	}
	if req.Options.ExecTimeout <= 0 {
		return nil, fmt.Errorf("exec_timeout 不能为空且必须大于 0")
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
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}

	if req.FilePath == "" {
		return nil, fmt.Errorf("file_path 不能为空")
	}
	if req.RemotePath == "" {
		return nil, fmt.Errorf("remote_path 不能为空")
	}
	if len(req.Nodes) == 0 {
		return nil, fmt.Errorf("nodes 数组不能为空")
	}
	if req.Options.ExecTimeout <= 0 {
		return nil, fmt.Errorf("exec_timeout 必须大于 0")
	}

	// seq 查重
	seen := make(map[int]bool, len(req.Nodes))
	for _, node := range req.Nodes {
		if seen[node.Seq] {
			return nil, fmt.Errorf("seq %d 重复", node.Seq)
		}
		seen[node.Seq] = true
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
