package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"SSHFleet/internal/core"
	"SSHFleet/internal/jsonproc"
	"SSHFleet/internal/localfs"
	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"

	"go.uber.org/zap"
)

// batchRequest 请求类型约束：三种请求都有 Options 字段
type batchRequest interface {
	jsonproc.ExecuteRequest | jsonproc.UploadRequest | jsonproc.DownloadRequest
}

// batchResult 结果类型约束：所有批量操作的结果都带 ConnectSuccess 字段
type batchResult interface {
	ssh.ExecResult | ssh.UploadResult
}

// batchRunner executor 统一接口，屏蔽三个 executor 的具体类型
type batchRunner[TTask any, TResult any] interface {
	Run(tasks []*TTask) <-chan *TResult
}

// batchOperation 批量操作描述符（配料表）：声明每个端点的差异点，骨架统一执行公共流程
type batchOperation[TReq any, TTask any, TResult any] struct {
	api              string // 接口名（writeParseError 错误前缀，保持与原来一致）
	logName          string // 中文日志名（"执行"/"上传"/"下载"）
	doneLog          string // 完成日志文案（"任务执行完成"/"上传任务完成"/"下载任务完成"）
	parse            func(body []byte) (*TReq, error)
	logParsed        func(req *TReq)
	prepare          func(w http.ResponseWriter, req *TReq) ([]*TTask, map[string]interface{}, bool) // tasks, initMsg, ok
	hasProgress      bool
	getOptions       func(req *TReq) jsonproc.Options                    // 提取并发配置
	isConnectSuccess func(result *TResult) bool                         // 判断节点连接成功
	makeExecutor     func(concurrency, total int, ctx context.Context, progressChan chan ssh.ProgressMsg) batchRunner[TTask, TResult]
}

// runBatch 泛型骨架：三个端点的公共流程只写一份
// acquire → readBody → parse → logParsed → prepare(校验+收集+构建) → setupSSE → init → progress → Run → 统计 → done → waitForShutdown
func runBatch[TReq batchRequest, TTask any, TResult batchResult](op batchOperation[TReq, TTask, TResult]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 一次性防护
		if !acquireRequestSlot(w, r) {
			return
		}

		log.Zlog.Info("收到" + op.logName + "请求，开始处理...")

		body, ok := readBody(w, r)
		if !ok {
			return
		}

		req, err := op.parse(body)
		if err != nil {
			writeParseError(w, op.api, err)
			return
		}
		op.logParsed(req)

		// 校验 + 收集 + 构建任务（失败已写错误响应并触发关服）
		tasks, initMsg, ok := op.prepare(w, req)
		if !ok {
			return
		}

		// 设置 SSE 响应（决策 C1：先 Flusher 检查、后设 header；Flush 由 WriteSSE 内部完成）
		if !setupSSE(w) {
			return
		}

		// 发送 init 消息（仅 upload；原行为：忽略写入错误，继续执行）
		if initMsg != nil {
			WriteSSE(w, initMsg)
		}

		// progress 通道与加锁写入（仅 upload/download）
		var progressChan chan ssh.ProgressMsg
		var progressWg *sync.WaitGroup
		writeSSE := func(data interface{}) error { return WriteSSE(w, data) }
		if op.hasProgress {
			progressChan = make(chan ssh.ProgressMsg, len(tasks)*10)
			var sseMu sync.Mutex
			base := writeSSE
			writeSSE = func(data interface{}) error {
				sseMu.Lock()
				defer sseMu.Unlock()
				return base(data)
			}
			progressWg = startProgressConsumer(progressChan, writeSSE)
		}

		// 使用独立的 context，不依赖 HTTP 请求（防止客户端断开导致所有任务终止）
		execCtx, execCancel := context.WithCancel(context.Background())
		defer execCancel()

		executor := op.makeExecutor(op.getOptions(req).Concurrency, len(tasks), execCtx, progressChan)
		resultChan := executor.Run(tasks)

		total := len(tasks)
		connSuccess, connFailed := 0, 0
		for result := range resultChan {
			if err := writeSSE(result); err != nil {
				log.Zlog.Error("SSE 写入失败", zap.Error(err))
				if op.hasProgress {
					close(progressChan)
				}
				return
			}
			if op.isConnectSuccess(result) {
				connSuccess++
			} else {
				connFailed++
			}
		}
		if op.hasProgress {
			close(progressChan)
			progressWg.Wait()
		}

		log.Zlog.Info("连接统计", zap.Int("total", total), zap.Int("connSuccess", connSuccess), zap.Int("connFailed", connFailed))

		done := ssh.DoneResponse{Type: "done", Total: total}
		// 原行为：忽略 done 写入错误，继续等待关闭信号并关服
		WriteSSE(w, done)
		log.Zlog.Succ(op.doneLog, zap.Int("total", total))

		// 等待客户端发送关闭信号，30秒超时防御
		waitForShutdown()
	}
}

// nodeToSSHConfig 节点信息 → SSH 配置（三端点共用）
func nodeToSSHConfig(node jsonproc.NodeInfo, opts jsonproc.Options) *ssh.SSHConfig {
	return &ssh.SSHConfig{
		IP:             node.IP,
		Port:           node.Port,
		User:           node.User,
		Password:       node.Password,
		KeyContent:     node.KeyContent,
		KeyPassphrase:  node.KeyPassphrase,
		ConnectTimeout: time.Duration(opts.ConnectTimeout) * time.Second,
		ExecTimeout:    time.Duration(opts.ExecTimeout) * time.Second,
	}
}

// ==================== 三张配料表 ====================

var executeOp = batchOperation[jsonproc.ExecuteRequest, core.SSHTask, ssh.ExecResult]{
	api:     "execute",
	logName: "执行",
	doneLog: "任务执行完成",
	parse:   jsonproc.ParseRequest,
	logParsed: func(req *jsonproc.ExecuteRequest) {
		log.Zlog.Info("执行请求解析成功",
			zap.String("command", req.Command),
			zap.Int("nodes", len(req.Nodes)),
			zap.Int("concurrency", req.Options.Concurrency),
			zap.Int("connectTimeout", req.Options.ConnectTimeout),
			zap.Int("execTimeout", req.Options.ExecTimeout))
	},
	prepare: func(w http.ResponseWriter, req *jsonproc.ExecuteRequest) ([]*core.SSHTask, map[string]interface{}, bool) {
		tasks := make([]*core.SSHTask, 0, len(req.Nodes))
		for _, node := range req.Nodes {
			tasks = append(tasks, &core.SSHTask{
				Seq:     node.Seq,
				Config:  nodeToSSHConfig(node, req.Options),
				Command: req.Command,
			})
		}
		return tasks, nil, true
	},
	hasProgress: false,
	getOptions: func(req *jsonproc.ExecuteRequest) jsonproc.Options {
		return req.Options
	},
	isConnectSuccess: func(result *ssh.ExecResult) bool {
		return result.ConnectSuccess
	},
	makeExecutor: func(concurrency, total int, ctx context.Context, _ chan ssh.ProgressMsg) batchRunner[core.SSHTask, ssh.ExecResult] {
		return core.NewBatchExecutor(concurrency, total, ctx)
	},
}

var uploadOp = batchOperation[jsonproc.UploadRequest, core.UploadTask, ssh.UploadResult]{
	api:     "upload",
	logName: "上传",
	doneLog: "上传任务完成",
	parse:   jsonproc.ParseUploadRequest,
	logParsed: func(req *jsonproc.UploadRequest) {
		log.Zlog.Info("上传请求解析成功",
			zap.String("filePath", req.FilePath),
			zap.String("remotePath", req.RemotePath),
			zap.Int("nodes", len(req.Nodes)),
			zap.Bool("sudo", req.Options.Sudo))
	},
	prepare: func(w http.ResponseWriter, req *jsonproc.UploadRequest) ([]*core.UploadTask, map[string]interface{}, bool) {
		// 决策 B7：file_path 必须绝对路径（拒绝相对路径，不再静默转绝对）
		if !filepath.IsAbs(req.FilePath) {
			writeError(w, http.StatusBadRequest, "INVALID_PATH", fmt.Sprintf("upload 路径校验失败: file_path 必须是绝对路径: %s", req.FilePath))
			go server.Shutdown(context.Background())
			return nil, nil, false
		}

		// 校验本地 file_path
		if _, err := os.Stat(req.FilePath); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_PATH", fmt.Sprintf("upload 路径校验失败: file_path 不存在或不可读: %s", req.FilePath))
			go server.Shutdown(context.Background())
			return nil, nil, false
		}

		// 收集文件清单（CollectFiles 内部 IsAbs 防御在此生效）
		fileItems, err := localfs.CollectFiles(req.FilePath)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_PATH", fmt.Sprintf("upload 文件清单收集失败: %s", err.Error()))
			go server.Shutdown(context.Background())
			return nil, nil, false
		}
		log.Zlog.Info("文件清单收集完成", zap.Int("count", len(fileItems)))

		// 计算每个节点的总字节数
		var totalBytesPerNode int64
		for _, item := range fileItems {
			totalBytesPerNode += item.FileSize
		}

		// init 消息
		initMsg := map[string]interface{}{
			"type":                 "init",
			"total_nodes":          len(req.Nodes),
			"total_bytes_per_node": totalBytesPerNode,
		}

		// 构建上传任务
		tasks := make([]*core.UploadTask, 0, len(req.Nodes))
		for _, node := range req.Nodes {
			tasks = append(tasks, &core.UploadTask{
				Seq:        node.Seq,
				Config:     nodeToSSHConfig(node, req.Options),
				FileItems:  fileItems,
				RemotePath: req.RemotePath,
				UseSudo:    req.Options.Sudo,
			})
		}
		return tasks, initMsg, true
	},
	hasProgress: true,
	getOptions: func(req *jsonproc.UploadRequest) jsonproc.Options {
		return req.Options
	},
	isConnectSuccess: func(result *ssh.UploadResult) bool {
		return result.ConnectSuccess
	},
	makeExecutor: func(concurrency, total int, ctx context.Context, progressChan chan ssh.ProgressMsg) batchRunner[core.UploadTask, ssh.UploadResult] {
		return core.NewBatchUploadExecutor(concurrency, total, ctx, progressChan)
	},
}

var downloadOp = batchOperation[jsonproc.DownloadRequest, core.DownloadTask, ssh.UploadResult]{
	api:     "download",
	logName: "下载",
	doneLog: "下载任务完成",
	parse:   jsonproc.ParseDownloadRequest,
	logParsed: func(req *jsonproc.DownloadRequest) {
		log.Zlog.Info("下载请求解析成功",
			zap.String("remotePath", req.RemotePath),
			zap.String("localPath", req.LocalPath),
			zap.Int("nodes", len(req.Nodes)),
			zap.Bool("sudo", req.Options.Sudo))
	},
	prepare: func(w http.ResponseWriter, req *jsonproc.DownloadRequest) ([]*core.DownloadTask, map[string]interface{}, bool) {
		// 决策 B4：remote_path 必须绝对路径（远程为 Linux 路径，以 / 开头）
		if !strings.HasPrefix(req.RemotePath, "/") {
			writeError(w, http.StatusBadRequest, "INVALID_PATH", fmt.Sprintf("download 路径校验失败: remote_path 必须是绝对路径: %s", req.RemotePath))
			go server.Shutdown(context.Background())
			return nil, nil, false
		}

		// 决策 B8：local_path 必须绝对路径（对称拒绝，不再静默转绝对）
		if !filepath.IsAbs(req.LocalPath) {
			writeError(w, http.StatusBadRequest, "INVALID_PATH", fmt.Sprintf("download 路径校验失败: local_path 必须是绝对路径: %s", req.LocalPath))
			go server.Shutdown(context.Background())
			return nil, nil, false
		}

		// 校验本地 local_path 是否存在且是目录
		if info, err := os.Stat(req.LocalPath); err != nil || !info.IsDir() {
			writeError(w, http.StatusBadRequest, "INVALID_PATH", fmt.Sprintf("download 路径校验失败: local_path 不存在或不是目录: %s", req.LocalPath))
			go server.Shutdown(context.Background())
			return nil, nil, false
		}

		// 构建下载任务
		tasks := make([]*core.DownloadTask, 0, len(req.Nodes))
		for _, node := range req.Nodes {
			tasks = append(tasks, &core.DownloadTask{
				Seq:        node.Seq,
				Config:     nodeToSSHConfig(node, req.Options),
				RemotePath: req.RemotePath,
				LocalPath:  req.LocalPath,
				UseSudo:    req.Options.Sudo,
			})
		}
		return tasks, nil, true
	},
	hasProgress: true,
	getOptions: func(req *jsonproc.DownloadRequest) jsonproc.Options {
		return req.Options
	},
	isConnectSuccess: func(result *ssh.UploadResult) bool {
		return result.ConnectSuccess
	},
	makeExecutor: func(concurrency, total int, ctx context.Context, progressChan chan ssh.ProgressMsg) batchRunner[core.DownloadTask, ssh.UploadResult] {
		return core.NewBatchDownloadExecutor(concurrency, total, ctx, progressChan)
	},
}
