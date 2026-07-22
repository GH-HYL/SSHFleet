## REMOVED Requirements

### Requirement: Python Fabric/SFTP upload path
旧的上传路径使用 Python Fabric + paramiko SFTP 库直接执行文件传输。

**Reason**: 上传功能已完全迁移到 Go 二进制（通过 `/api/v1/upload` HTTP 接口），旧代码为死代码。
**Migration**: 无需迁移，上传功能已通过 Go 路径正常工作。

#### Scenario: Upload via old SFTP path
- **WHEN** 用户执行 `-u` 上传命令
- **THEN** 系统 SHALL 通过 Go 二进制的 `/api/v1/upload` 接口执行上传（而非 Python Fabric SFTP）

### Requirement: Python Fabric/SFTP download path
旧的下载路径使用 Python Fabric + paramiko SFTP 库直接执行文件下载。

**Reason**: 用户确认不再需要 `-d` 下载功能。
**Migration**: 无替代方案，功能已移除。

#### Scenario: Download via old SFTP path
- **WHEN** 用户尝试使用 `-d` 参数
- **THEN** 系统 SHALL 报错提示该参数不再支持

### Requirement: -d download parameter
`-d` 参数允许用户指定远程文件/目录路径进行下载。

**Reason**: 用户确认不再需要下载功能。
**Migration**: 无替代方案，功能已移除。

#### Scenario: -d parameter usage
- **WHEN** 用户在命令行中使用 `-d` 参数
- **THEN** 系统 SHALL 显示未知参数错误

### Requirement: fabric/paramiko/invoke dependencies
项目依赖 fabric、paramiko、invoke 三个 Python 包用于 SSH 连接和 SFTP 传输。

**Reason**: 这些依赖仅被旧的 transfer 路径使用，删除后不再需要。
**Migration**: 无需替代，SSH 连接由 Go 端处理。

#### Scenario: Dependency removal
- **WHEN** 旧 transfer 代码被删除
- **THEN** requirements.txt 中 SHALL 不再包含 fabric、paramiko、invoke 依赖
