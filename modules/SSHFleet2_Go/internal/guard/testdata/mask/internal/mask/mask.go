package mask

// 正样本：遮法住在本包，星号写在这里不报。

const token = "****"

// Secret 遮一个值。
func Secret(s string) string {
	if len(s) <= 4 {
		return token
	}
	return s[:2] + token + s[len(s)-2:]
}
