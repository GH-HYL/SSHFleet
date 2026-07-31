package jsonproc

import (
	"encoding/json"
	"testing"
)

func TestParseDownloadRequest_Valid(t *testing.T) {
	data := []byte(`{
		"remote_path": "/opt/logs/app.log",
		"local_path": "/home/user/downloads",
		"options": {
			"concurrency": 5,
			"connect_timeout": 10,
			"exec_timeout": 300
		},
		"nodes": [
			{"seq": 0, "ip": "10.0.0.1", "port": 22, "user": "root", "password": "pass"},
			{"seq": 1, "ip": "10.0.0.2", "port": 22, "user": "root", "password": "pass"},
			{"seq": 2, "ip": "10.0.0.3", "port": 22, "user": "root", "password": "pass"},
			{"seq": 3, "ip": "10.0.0.4", "port": 22, "user": "root", "password": "pass"},
			{"seq": 4, "ip": "10.0.0.5", "port": 22, "user": "root", "password": "pass"}
		]
	}`)

	req, err := ParseDownloadRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.RemotePath != "/opt/logs/app.log" {
		t.Errorf("expected RemotePath='/opt/logs/app.log', got '%s'", req.RemotePath)
	}
	if req.LocalPath != "/home/user/downloads" {
		t.Errorf("expected LocalPath='/home/user/downloads', got '%s'", req.LocalPath)
	}
	if req.Options.Concurrency != 5 {
		t.Errorf("expected Concurrency=5, got %d", req.Options.Concurrency)
	}
	if req.Options.ExecTimeout != 300 {
		t.Errorf("expected ExecTimeout=300, got %d", req.Options.ExecTimeout)
	}
	if len(req.Nodes) != 5 {
		t.Errorf("expected 5 nodes, got %d", len(req.Nodes))
	}
}

func TestParseDownloadRequest_EmptyRemotePath(t *testing.T) {
	data := []byte(`{
		"remote_path": "",
		"local_path": "/home/user/downloads",
		"options": {"exec_timeout": 300},
		"nodes": [{"seq": 0, "ip": "10.0.0.1", "user": "root", "password": "pass"}]
	}`)

	_, err := ParseDownloadRequest(data)
	if err == nil {
		t.Fatal("expected error for empty remote_path")
	}
}

func TestParseDownloadRequest_EmptyLocalPath(t *testing.T) {
	data := []byte(`{
		"remote_path": "/opt/logs/app.log",
		"local_path": "",
		"options": {"exec_timeout": 300},
		"nodes": [{"seq": 0, "ip": "10.0.0.1", "user": "root", "password": "pass"}]
	}`)

	_, err := ParseDownloadRequest(data)
	if err == nil {
		t.Fatal("expected error for empty local_path")
	}
}

func TestParseDownloadRequest_EmptyNodes(t *testing.T) {
	data := []byte(`{
		"remote_path": "/opt/logs/app.log",
		"local_path": "/home/user/downloads",
		"options": {"exec_timeout": 300},
		"nodes": []
	}`)

	_, err := ParseDownloadRequest(data)
	if err == nil {
		t.Fatal("expected error for empty nodes")
	}
}

func TestParseDownloadRequest_ZeroExecTimeout(t *testing.T) {
	data := []byte(`{
		"remote_path": "/opt/logs/app.log",
		"local_path": "/home/user/downloads",
		"options": {"exec_timeout": 0},
		"nodes": [{"seq": 0, "ip": "10.0.0.1", "user": "root", "password": "pass"}]
	}`)

	_, err := ParseDownloadRequest(data)
	if err == nil {
		t.Fatal("expected error for zero exec_timeout")
	}
}

func TestParseDownloadRequest_DuplicateSeq(t *testing.T) {
	data := []byte(`{
		"remote_path": "/opt/logs/app.log",
		"local_path": "/home/user/downloads",
		"options": {"exec_timeout": 300},
		"nodes": [
			{"seq": 0, "ip": "10.0.0.1", "user": "root", "password": "pass"},
			{"seq": 0, "ip": "10.0.0.2", "user": "root", "password": "pass"}
		]
	}`)

	_, err := ParseDownloadRequest(data)
	if err == nil {
		t.Fatal("expected error for duplicate seq")
	}
}

func TestParseDownloadRequest_DefaultValues(t *testing.T) {
	data := []byte(`{
		"remote_path": "/opt/logs/app.log",
		"local_path": "/home/user/downloads",
		"options": {"exec_timeout": 300},
		"nodes": [
			{"seq": 0, "ip": "10.0.0.1", "user": "root", "password": "pass"},
			{"seq": 1, "ip": "10.0.0.2", "user": "root", "password": "pass"}
		]
	}`)

	req, err := ParseDownloadRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Concurrency defaults to len(nodes) when <= 0
	if req.Options.Concurrency != 2 {
		t.Errorf("expected default Concurrency=2, got %d", req.Options.Concurrency)
	}
	// ConnectTimeout defaults to 10
	if req.Options.ConnectTimeout != 10 {
		t.Errorf("expected default ConnectTimeout=10, got %d", req.Options.ConnectTimeout)
	}
	// Port defaults to 22
	for _, node := range req.Nodes {
		if node.Port != 22 {
			t.Errorf("expected default Port=22 for %s, got %d", node.IP, node.Port)
		}
	}
}

func TestParseDownloadRequest_InvalidJSON(t *testing.T) {
	_, err := ParseDownloadRequest([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDownloadRequest_JSON(t *testing.T) {
	req := DownloadRequest{
		RemotePath: "/opt/logs/app.log",
		LocalPath:  "/home/user/downloads",
		Options: Options{
			Concurrency:    5,
			ConnectTimeout: 10,
			ExecTimeout:    300,
		},
		Nodes: []NodeInfo{
			{Seq: 0, IP: "10.0.0.1", Port: 22, User: "root", Password: "pass"},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded DownloadRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decoded.RemotePath != req.RemotePath {
		t.Errorf("expected RemotePath='%s', got '%s'", req.RemotePath, decoded.RemotePath)
	}
	if decoded.LocalPath != req.LocalPath {
		t.Errorf("expected LocalPath='%s', got '%s'", req.LocalPath, decoded.LocalPath)
	}
}
