package ask

// Ask 引了个模块外的包——类二的坏样本（零第三方依赖）。
func Ask() string { return thirdparty.Name() }
