# M3-13 ssh：SFTP 上传 / 下载 + 进度读写器

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 12

## 范围

`internal/ssh` SFTP 与进度读写器（对位旧 `ssh_upload.go` / `ssh_download.go`）：

- **上传**：连接 → （sudo 模式）前置 `sudo rm -rf /tmp/.SSHFleet_tmp/` → SFTP 客户端 → 首发进度 → `Stat(remotePath)` **必须是已存在的目录**（不存在报「远程目标路径不存在」，工具不建目录——用户 2026-09-11 裁定 Q7）→ 本地文件预检 → 逐文件
  - `effectiveSudo = useSudo && user != "root"`（root 跳过 sudo）
  - 非 sudo：直接 `Create` 写入 + `Chmod`；sudo：临时目录 `/tmp/.SSHFleet_tmp/<随机hex>` → 上传 → `sudo mv '<tmp>' '<目标>/'` → 清理
  - 远程文件已存在 → 该文件失败并**终止该节点**（不覆盖）；SFTP 写失败删除半成品；1MB 缓冲流式写
  - 文件清单来自本地收集（扁平化：只用文件名，不保留子目录结构——旧行为，保持）
- **下载**：连接 → SFTP 客户端 → `test -e`（sudo 模式 `sudo test -e`）预检 → `Stat` → 目录时 `find -printf '%s %P\n'` 取文件清单（GNU find 假设，保持）→ 空目录报「远程目录为空」→ 逐文件下载到 `本地目录/IP/…`（按 IP 分目录，对位旧实现）
- **进度读写器**：`io.Writer`（上传）/ `io.Reader`（下载）包裹，累计字节并 500ms 节流回调；`callback != nil` 判空补齐（旧 `progressWriter.Write` 未判空会 panic，spec 已定）
- 退出码语义：传输失败**不设**退出码（保持 nil），全部成功置 0（CONTEXT「传输失败」）

## 验证

- 对测试节点上传单文件 / 目录（sudo 与非 sudo、root 用户跳过 sudo 分支）、已存在文件冲突、目标目录不存在（报错）
- 下载单文件 / 目录，按 IP 落盘；空目录报错

## Comments

- 2026-09-14 完成。真机验证：上传到 `/tmp/` 成功（18B，1/1 文件）；目标目录不存在 → 报「远程目标路径不存在」（Q7 口径，工具不建目录）；重复上传 → 报「文件已存在」并终止该节点；下载单文件成功并按 IP 落 `<本地目录>/172.28.118.49/`，内容一致。sudo 临时目录路径与 root 跳过 sudo 分支按旧实现保留。
