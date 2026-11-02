package contract

// Node 是「对谁做」那一格里的一台机器：连接要素五样，外加凭据三样。
//
// 字段到这里时已经按「清单优先、配置兜底」补全过（词表「字段补全」），下游不再回头
// 看清单或配置——那是解析站的活。加密开着时 Password 与 KeyPassword 是**密文文件的
// 相对名**，由凭据包解出明文后换掉；别处不该假设它已经是密码。
type Node struct {
	IP   string
	Port string
	User string

	// Password 是这一台的登录密码；加密开着时它是密文文件相对名的原值。
	Password string

	// Key 是私钥文件路径；KeyPassword 是它的口令。
	// 两者**同源**取用：清单第 5 列配的私钥配第 6 列的口令，配置里的私钥配配置里的口令。
	Key         string
	KeyPassword string
}
