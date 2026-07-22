---
comet_change: defer-password-validation
role: technical-design
canonical_spec: openspec
archived-with: 2026-07-16-defer-password-validation
status: final
---

# 密码文件延迟验证技术设计

## 1. 概述

将密码文件验证逻辑从 `yaml.py:load_config` 移动到 `core.py:read_nodes_infos`，实现延迟验证。

## 2. 技术方案

### 2.1 yaml.py 修改

**删除内容**（第 97-116 行）：
```python
# 读取密码文件
password_path = config_dict["account"]["password"]
if password_path:
    password_path = os.path.expanduser(password_path)
    if not os.path.exists(password_path):
        raise FileNotFoundError(f"密码文件 {password_path} 不存在")

    with open(password_path, "r", encoding="utf-8") as f:
        password_b64 = f.read().strip()

    # 验证 base64 格式
    try:
        base64.b64decode(password_b64)
    except Exception as e:
        print(
            f"\033[91m[ERROR]\033[0m\033[93m [function:load_config]\033[0m 密码文件内容不是有效的base64编码\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )
        sys.exit(1)

    config_dict["account"]["password"] = password_b64
```

**保留内容**：
```python
# 读取密码文件路径（不验证）
password_path = config_dict["account"]["password"]
if password_path:
    config_dict["account"]["password"] = os.path.expanduser(password_path)
```

### 2.2 core.py 新增函数

```python
def validate_password_file(file_path: str) -> None:
    """
    验证密码文件的有效性
    
    Args:
        file_path: 密码文件路径
        
    Raises:
        SystemExit: 验证失败时退出程序
    """
    # 1. 检查文件是否存在
    if not os.path.exists(file_path):
        utils.print_error_information_and_exit(
            "validate_password_file",
            f"密码文件不存在：{file_path}"
        )
    
    # 2. 检查文件是否可读
    try:
        with open(file_path, "r", encoding="utf-8") as f:
            content = f.read().strip()
    except PermissionError:
        utils.print_error_information_and_exit(
            "validate_password_file",
            f"密码文件无法读取：{file_path}"
        )
    except Exception as e:
        utils.print_error_information_and_exit(
            "validate_password_file",
            f"读取密码文件失败：{file_path}\n异常信息：{e}"
        )
    
    # 3. 检查文件内容是否为空
    if not content:
        utils.print_error_information_and_exit(
            "validate_password_file",
            f"密码文件内容为空：{file_path}"
        )
    
    # 4. 检查是否为有效的 Base64 编码
    try:
        decoded = base64.b64decode(content)
    except Exception as e:
        utils.print_error_information_and_exit(
            "validate_password_file",
            f"密码文件内容不是有效的 Base64 编码：{file_path}\n异常信息：{e}"
        )
    
    # 5. 检查解码后是否为空
    if not decoded:
        utils.print_error_information_and_exit(
            "validate_password_file",
            f"密码文件解码后内容为空：{file_path}"
        )
```

### 2.3 core.py 集成验证

在 `read_nodes_infos` 函数的密码处理分支（第 291-295 行）添加验证：

```python
elif (
    config.account.password and config.account.password != "None"
):  # 使用配置的默认值
    # 验证密码文件有效性
    validate_password_file(config.account.password)
    # 配置文件中的密码是经过base64编码的，需要解码
    password = base64.b64decode(config.account.password).decode("utf-8")
```

## 3. 数据流

```
用户运行程序
    ↓
yaml.py:load_config
    ↓ 读取配置文件
    ↓ 保留 password_path（不验证）
    ↓
core.py:read_nodes_infos
    ↓ 解析 CSV 文件
    ↓ 检查密码优先级
    ├─ CSV 有密码 → 使用 CSV 密码（跳过验证）
    ├─ 用户输入密码 → 使用输入密码（跳过验证）
    └─ 使用配置文件密码
        ↓
        validate_password_file(config.account.password)
        ↓ 验证文件有效性
        ↓ 解码 Base64
        ↓
        继续执行
```

## 4. 错误场景处理

| 场景 | 错误信息 | 处理方式 |
|------|----------|----------|
| 文件不存在 | 密码文件不存在：<路径> | 退出程序 |
| 文件不可读 | 密码文件无法读取：<路径> | 退出程序 |
| 内容为空 | 密码文件内容为空：<路径> | 退出程序 |
| 非 Base64 | 密码文件内容不是有效的 Base64 编码：<路径> | 退出程序 |
| 解码为空 | 密码文件解码后内容为空：<路径> | 退出程序 |

## 5. 测试策略

### 5.1 单元测试

```python
# test_validate_password_file.py
def test_file_not_exists():
    """测试文件不存在场景"""
    
def test_file_not_readable():
    """测试文件不可读场景"""
    
def test_file_empty():
    """测试文件内容为空场景"""
    
def test_invalid_base64():
    """测试非 Base64 编码场景"""
    
def test_empty_decoded():
    """测试解码后为空场景"""
    
def test_valid_password_file():
    """测试有效密码文件"""
```

### 5.2 集成测试

- 测试 CSV 密码优先级逻辑
- 测试用户输入密码优先级逻辑
- 测试配置文件密码验证逻辑

### 5.3 手动测试

1. 创建有效密码文件，测试正常流程
2. 创建无效密码文件，测试错误处理
3. 删除密码文件，测试不存在场景
4. 使用 CSV 密码，验证跳过验证

## 6. 风险缓解

**风险 1**: 用户运行时才发现密码文件问题
→ **缓解**: 错误信息明确，提供修复建议

**风险 2**: 循环中多次验证同一文件
→ **缓解**: 密码文件很小，性能影响可忽略

**风险 3**: Base64 解码失败的异常处理
→ **缓解**: 捕获所有异常，提供详细错误信息

## 7. 向后兼容性

- 功能行为不变，仅验证时机改变
- 配置文件格式不变
- 错误信息格式统一
- 无破坏性变更
