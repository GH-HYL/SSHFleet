# 05: output 4 处、ssh 1 处、batch 3 处、main.go 1 处迁到构造点

**要做的**：归档/输出 4 处、触发词正则 1 处、批量执行 3 处、以及 `main.go` 里改密入场信号那道守卫，共 9 处改走构造点。其中批量执行那两处是「内部错误」（第三行恒定「这是内部错误，请把本次命令反馈给作者」），走内部错误那个名字，调用点只给前两行。

Status: ready-for-agent
Blocked by: 01

- [ ] 门无命中：`grep -rnE --include='*.go' '\\n原因：|"[[:space:]]*原因：' internal/output/ internal/ssh/ internal/batch/ main.go | grep -v _test` 输出为空
- [ ] 内部错误那两处的第三行由构造点产出，调用点不再各写一遍那句话
- [ ] `output` / `ssh` 报错文本逐字节不变（既有断言一条未改、全绿）
- [ ] `batch` 无测试文件，人工触发一次上传并核对输出：缺清单那处的文字与改前一致
- [ ] `main.go` 守卫人工触发一次（临时移走「密码过期」分类的关键词，跑一次改密），三行输出与改前一致
- [ ] `go test ./internal/output ./internal/ssh` 全绿；`go build ./...` 通过

## Comments

- 2026-10-27：按 ADR-0028 建立。本工单是唯一需要人工触发核对的一批（`batch` 没有测试），验收项因此写死触发方式。
