package ask

// Ask 引了个模块外的包——类二的坏样本（不在点名白名单里）。
func Ask() string { return thirdparty.Name() }
