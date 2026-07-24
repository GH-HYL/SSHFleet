## Task 6: 测试验证

**Status**: DONE

### Test Results

| # | 测试场景 | 结果 |
|---|---------|------|
| 1 | 相对路径 (`./pw.txt`) → 与 `password_dir` 拼接 | PASS |
| 2 | 绝对路径 (`/home/user/pw.txt`) → 直接使用 | PASS |
| 3 | `~` 开头 (`~/pw.txt`) → 展开 HOME | PASS |
| 4 | 带空格路径 → strip 后正确处理 | PASS |
| 5 | 相对路径 + `password_dir` 未配置 → SystemExit | PASS |
| 6 | 所有密码路径有效 → 验证通过 | PASS |
| 7 | 密码文件不存在 → SystemExit + 错误信息 | PASS |
| 8 | 非法 Base64 编码 → SystemExit + 错误信息 | PASS |
| 9 | 密码列为空 + 有默认密码 → 使用默认密码 | PASS |
| 10 | 密码列为空 + 无默认密码 → SystemExit | PASS |
| 11 | 混合使用（路径 + 空值 + 默认密码）→ 通过 | PASS |
| 12 | 空密码文件 → SystemExit | PASS |
| 13 | 绝对路径密码文件 → 通过 | PASS |
| 14 | Account 模型包含 `password_dir` 字段 | PASS |
| 15 | `password_dir` 类型为 str | PASS |
| 16 | 实际配置文件加载成功，包含 `password_dir` | PASS |

### 编译验证

- `src/input/csv.py` — 编译通过
- `src/yaml.py` — 编译通过
- `src/input/args.py` — 编译通过
- `src/check/files.py` — 编译通过

### 导入验证

- `SSHFleetConfig`, `load_config` — OK
- `resolve_password_path`, `validate_csv_passwords`, `read_nodes_infos` — OK
- `validate_password_file` — OK

### Issues Found

None. All 16/16 tests passed, all files compile cleanly, all imports resolve correctly.

### Verification Summary

Tasks 1-5 的密码存储变换实现完整且正确：
- **Task 1**: `SSHFleet.yaml` 新增 `password_dir` 字段 ✓
- **Task 2**: `Account` 模型新增 `password_dir` 字段，`load_config` 中 `expanduser` 处理 ✓
- **Task 3**: `resolve_password_path` 函数正确处理相对路径、绝对路径、`~` 展开 ✓
- **Task 4**: `validate_password_file` 函数验证文件存在性、可读性、非空、Base64 有效性 ✓
- **Task 5**: `validate_csv_passwords` 预检查 + `read_nodes_infos` 集成密码文件读取 ✓
