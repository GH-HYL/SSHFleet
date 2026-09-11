# M1-02 internal/config：TOML 配置加载 + 模板

Type: task
Status: resolved
Resolved: 2026-09-11
Blocked by: 01

## 范围

`internal/config`：

- 加载 `./config/SSHFleet.conf`，基准为**当前工作目录**（D28）
- BurntSushi/toml 解码；解码后取未识别键集合，报错**列出具体键名**（未知字段零容忍）
- 字段基准 = 旧 YAML 配置（到 `modules/SSHFleet_bak/` 按需查阅），差异仅 spec D6 列表：
  - 删 `paths.exe` 段（双进程消亡）
  - `paths.keywords.*` / `paths.logs.*` / `paths.files.*` 三段两层并入单段 `[paths]`，字段名直写
  - `password_security` 为 `int`，取值只允许 1 / 2 / 3
  - 必填性：超时值与开关类给默认值（10 / 60 / 300 / true）；账号类保持必填（`port` / `user` / `secret_dir` / `password`）
- 规则文件（dangerous_keywords / error_keywords）转 TOML 属 M4，本工单不做

工程根 `config/SSHFleet.conf` 模板写全字段。

## 验证

- 缺必填字段报错且指明字段；未知键报错列出键名；默认值生效；模板可直接加载
- 与旧 YAML 逐字段对照无遗漏，差异仅为 D6 列表

## Comments

- 2026-09-11 完成。必填性以 md.IsDefined 逐键判定、缺项一次列全；未知键经 BurntSushi 未识别键集合报错列出键名；password_security int 校验 1/2/3；凭据三字段按 D31 统一顺序解析。模板 `config/SSHFleet.conf` 落工程根（paths.exec 默认名去 `_Go`：`SSHFleetExec.log`，随 D36）。冒烟验证：模板可加载、缺字段/未知键均报错。