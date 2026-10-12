# M2-08 credential：加解密引擎 + 凭据读盘校验

Type: task
Status: resolved
Resolved: 2026-09-11
Blocked by: 07

## 范围

`internal/credential`：

- **0x01 旧格式：只读解密**——SHA256 派生双钥（seed+"enc"/"mac"）+ 计数器密钥流 XOR（块 32B，计数器 8B 大端）+ HMAC-SHA256，逐字节对齐旧 cipher.py
- **0x02 新格式：AES-256-GCM**（crypto/aes + crypto/cipher），密钥派生 stdlib `crypto/hkdf`；布局 `base64(0x02 || 12B nonce || ct||tag)`
- 结构识别：classify（encrypted 优先于 base64）、isProbablyBase64Text（重编码回验）、looksEncrypted（0x01/0x02 双版本）
- 凭据读盘校验（对位旧 credential.py 深模块）：错误码自定义字符串类型 + 常量（实现途径 8）；错误码 → 短文案 / 完整指引两张文案表照搬
- 解码缓存**不移植**（spec 实现层差异）：解码发生在预检、结果直接进节点数据（M2-10 消费）

## 验证

- 临时 Go test：Python 旧引擎生成 0x01 token → Go 解密一致；0x02 往返；classify 分支——跑完删除测试文件

## Comments

- 2026-09-11 完成。临时测试验证后已删除：**旧 Python 引擎生成的 0x01 密文在 Go 侧解密逐字节一致**（TOKEN=AYeqyGJ7Jx7x… → pw123456），0x02 AES-256-GCM 往返、错误密钥拒绝、版本互斥（0x01 密文不被 V2 解密器接受）全通过。HKDF info = `SSHFleet-credential-encryption-v2`，nonce 12B，布局 `base64(02 || nonce || ct||tag)`。
