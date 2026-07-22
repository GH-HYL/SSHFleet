---
change: defer-password-validation
design-doc: docs/superpowers/specs/2026-07-16-defer-password-validation-design.md
base-ref: 5251860af7a2876bc4daef3e3d0f4bd36c36e368
status: pending
created: 2026-07-16
archived-with: 2026-07-16-defer-password-validation
---

# 密码文件延迟验证实现计划

## 1. 概述

将密码文件验证逻辑从 `yaml.py:load_config` 移动到 `core.py:read_nodes_infos`，实现延迟验证。

**目标**：
- 仅在实际使用配置文件密码时才执行验证
- 增强密码文件检查，提供更全面的错误提示
- 保持向后兼容，功能行为不变

## 2. 实现任务分解

### 任务 1：修改 yaml.py - 移除密码验证逻辑

**文件**: `SSHFleet_py/src/yaml.py`
**行数**: 97-116
**变更类型**: 删除 + 修改

#### 1.1 删除密码文件验证代码

删除第 97-116 行的密码文件验证逻辑：
```python
# 删除以下代码块
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

#### 1.2 保留密码路径读取

替换为简单的路径展开（不验证）：
```python
# 读取密码文件路径（不验证）
password_path = config_dict["account"]["password"]
if password_path:
    config_dict["account"]["password"] = os.path.expanduser(password_path)
```

**验证点**：
- [x] `config_dict["account"]["password"]` 保持原始路径值（展开后）
- [x] 不再执行文件存在性检查
- [x] 不再读取文件内容
- [x] 不再验证 Base64 格式

---

### 任务 2：在 core.py 添加密码文件验证函数

**文件**: `SSHFleet_py/src/core.py`
**位置**: 文件顶部区域（导入语句后）
**变更类型**: 新增函数

#### 2.1 创建 validate_password_file 函数

在 `core.py` 中添加以下函数：

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

**验证点**：
- [x] 函数接收 `file_path: str` 参数
- [x] 检查文件存在性
- [x] 检查文件可读性（捕获 PermissionError）
- [x] 检查文件内容非空
- [x] 验证 Base64 编码有效性
- [x] 验证解码后内容非空
- [x] 所有错误使用 `utils.print_error_information_and_exit` 处理

---

### 任务 3：在 read_nodes_infos 中集成验证

**文件**: `SSHFleet_py/src/core.py`
**位置**: 第 291-295 行（密码处理分支）
**变更类型**: 修改

#### 3.1 在密码处理分支添加验证调用

修改第 291-295 行的密码处理逻辑：

**原代码**：
```python
elif (
    config.account.password and config.account.password != "None"
):  # 使用配置的默认值
    # 配置文件中的密码是进过base64编码的，需要解码
    password = base64.b64decode(config.account.password).decode("utf-8")
```

**修改为**：
```python
elif (
    config.account.password and config.account.password != "None"
):  # 使用配置的默认值
    # 验证密码文件有效性
    validate_password_file(config.account.password)
    # 配置文件中的密码是经过base64编码的，需要解码
    password = base64.b64decode(config.account.password).decode("utf-8")
```

**验证点**：
- [x] 仅在需要使用配置文件密码时才调用 `validate_password_file`
- [x] CSV 密码优先时跳过验证（第 289-290 行）
- [x] 用户手动输入密码时跳过验证（第 296-324 行）
- [x] 配置文件密码被使用时执行验证

---

### 任务 4：测试验证

#### 4.1 单元测试

创建 `test_validate_password_file.py` 测试文件：

```python
# 测试用例
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

#### 4.2 集成测试

- 测试 CSV 密码优先级逻辑
- 测试用户输入密码优先级逻辑
- 测试配置文件密码验证逻辑

#### 4.3 手动测试

1. 创建有效密码文件，测试正常流程
2. 创建无效密码文件，测试错误处理
3. 删除密码文件，测试不存在场景
4. 使用 CSV 密码，验证跳过验证

---

## 3. 数据流变化

### 修改前
```
用户运行程序
    ↓
yaml.py:load_config
    ↓ 读取配置文件
    ↓ 读取密码文件
    ↓ 验证文件存在性
    ↓ 验证 Base64 格式
    ↓ 读取文件内容
    ↓ 返回 config_dict
    ↓
core.py:read_nodes_infos
    ↓ 使用已验证的密码
```

### 修改后
```
用户运行程序
    ↓
yaml.py:load_config
    ↓ 读取配置文件
    ↓ 展开密码文件路径（不验证）
    ↓ 返回 config_dict
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
        ↓ 验证 Base64 格式
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

## 5. 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| 用户运行时才发现密码文件问题 | 错误信息明确，提供修复建议 |
| 循环中多次验证同一文件 | 密码文件很小，性能影响可忽略 |
| Base64 解码失败的异常处理 | 捕获所有异常，提供详细错误信息 |

## 6. 向后兼容性

- ✅ 功能行为不变，仅验证时机改变
- ✅ 配置文件格式不变
- ✅ 错误信息格式统一
- ✅ 无破坏性变更

## 7. 依赖项

- 无新增依赖
- 复用现有 `utils.print_error_information_and_exit` 函数
- 复用现有 `base64` 模块

## 8. 完成标准

- [x] yaml.py 中密码验证逻辑已删除
- [x] core.py 中 validate_password_file 函数已添加
- [x] read_nodes_infos 中已集成验证调用
- [x] 所有测试用例通过
- [x] 手动测试验证延迟验证行为
- [x] 错误信息清晰易懂
