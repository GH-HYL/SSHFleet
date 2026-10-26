package nodelist

import (
	"errors"
	"fmt"
	"strings"

	"sshfleet/internal/cli"
	"sshfleet/internal/config"
	"sshfleet/internal/credential"
)

// precheckCredentials 凭据预检：校验与取值一次完成（值按加密开关解释），结果随行返回。
// 全部通过才进入逐节点解析——保证交互提示不会发生在凭据错误暴露之前。
//
// 检查范围随 -k 变：
//   - 不带 -k：只查第 1–4 列
//   - 带 -k：额外查第 5 列（私钥必须可用；清单与配置都没有私钥时报错退出）
//   - 第 6 列的**内容**永不检查：工具无法判定私钥是否加密，报了就是误报，只能靠用户自觉。
//     唯一的例外是配置缺项——那格写了相对名却没配 secret_dir 时按行报出来，因为那不是
//     「口令对不对」的问题，而是它压根不知道该去哪个目录找。
//
// 同一凭据在一次预检内只读一次：清单多行常共用同一个密码文件，逐行读会重复读盘解密 N 次，
// 并在内存里留下 N 份相同明文；命中缓存的行直接复用首次结果。
func precheckCredentials(rows [][]string, args *cli.Args, cfg *config.Config) (*precheckResult, error) {
	encrypted := cfg.Credential.Encrypt

	pre := &precheckResult{rows: make([]rowCreds, len(rows))}
	cache := newCredCache()
	var errs []string

	for idx, raw := range rows {
		row := padRow(raw)
		rc := rowCreds{}

		passwordRaw := strings.TrimSpace(row[3])
		// 第 5 列**原值**直交系统：不去空白、不拼 secret_dir、不展开 ~——
		// 照 --convert-secret 的做法，读到什么就按什么找，找不到时把系统原因报出来。
		keyRaw := row[4]

		// 私钥：写了 -k 才用密钥登录，路径取「清单第 5 列 > 配置 account.key」
		key := ""
		if args.Key {
			key = keyRaw
			if key == "" {
				key = cfg.Credential.Key
				rc.keyFromConfig = key != ""
			}
			if key == "" {
				errs = append(errs, fmt.Sprintf("行 %d (IP: %s): 用了 -k，但没有找到可用的私钥——在第 5 列写私钥路径，或把统一私钥配到配置 credential.key", idx+1, row[0]))
				pre.rows[idx] = rc
				continue
			}
		} else if keyRaw != "" {
			// 没加 -k 却在清单里配了私钥：配了没用上，值得提醒一句（配置文件里的默认值不提醒）
			pre.unusedKeyRows = append(pre.unusedKeyRows, idx+1)
		}
		rc.hasKey = key != ""

		// 密码列：取值 + 校验
		if passwordRaw != "" {
			spec, missingDir := credentialValueSpec(passwordRaw, encrypted, cfg.Credential.SecretDir)
			if missingDir {
				errs = append(errs, relativeColumnError(idx, row[0], "密码", passwordRaw))
				pre.rows[idx] = rc
				continue
			}
			plain, problems, fatalErr := cache.password(spec, encrypted, true)
			if fatalErr != nil {
				return nil, fatalErr
			}
			if len(problems) > 0 {
				errs = append(errs, prefixProblems(rowPrefix(idx, row[0], "密码"), problems)...)
				pre.rows[idx] = rc
				continue
			}
			rc.passwordPlain = plain
		} else if !rc.hasKey {
			// 密码列和密钥列均为空：依赖配置中的默认密码
			pre.needDefaultPassword = true
		}

		// 私钥内容：PEM 校验 + 内容捕获
		if key != "" {
			// 第 5 列（以及配置里的 credential.key）是私钥路径，不归 secret_dir 管：
			// 原值直接交给 os，短名字就是当前目录下的那个文件。
			keyPlain, keyCodes := cache.keyPEM(key)
			if len(keyCodes) > 0 {
				for _, ce := range keyCodes {
					line := rowPrefix(idx, row[0], "密钥") + credential.CredErrorLabel(ce.Code, key, ce.Detail)
					if ce.Code == credential.CodeMissing {
						// 找不到时补一句去路：这一列不拼 secret_dir，写相对名就是按当前目录找，
						// 与「配置里有个凭据目录」没有关系（ADR-0025 决定 4）。
						line += "（提示：第 5 列不拼 secret_dir，写本机绝对路径最稳妥）"
					}
					errs = append(errs, line)
				}
				pre.rows[idx] = rc
				continue
			}
			rc.keyContent = keyPlain
			pre.anyNodeUsesKey = true
			if rc.keyFromConfig {
				pre.anyNodeUsesConfigKey = true
			}

			// 第 6 列：口令只与「清单私钥」成对；私钥取自配置时第 6 列不参与。
			// 这一列永不检查——工具无法判定私钥是否加密，报了就是误报；取不到就当没给口令，
			// 真正解析私钥时若确实需要口令，会在那一步说清（见 ssh 侧的口令缺失报错）。
			if !rc.keyFromConfig {
				passRaw := strings.TrimSpace(row[5])
				if passRaw != "" {
					spec, missingDir := credentialValueSpec(passRaw, encrypted, cfg.Credential.SecretDir)
					if missingDir {
						errs = append(errs, relativeColumnError(idx, row[0], "口令", passRaw))
						pre.rows[idx] = rc
						continue
					}
					plain, _, fatalErr := cache.password(spec, encrypted, false)
					if fatalErr != nil {
						return nil, fatalErr
					}
					rc.keyPassRaw = plain
				}
			}
		}
		pre.rows[idx] = rc
	}

	// 有空密码行：验证一次默认密码（同时完成取值）
	if pre.needDefaultPassword {
		if cfg.Account.Password == "" {
			errs = append(errs, "密码列有空值，但 config 未配置默认密码(account.password)")
		} else {
			plain, problems, fatalErr := cache.password(cfg.Account.Password, encrypted, true)
			if fatalErr != nil {
				return nil, fatalErr
			}
			errs = append(errs, prefixProblems("默认密码", problems)...)
			if len(problems) == 0 {
				pre.defaultPasswordPlain = plain
			}
		}
	}

	// 配置私钥口令：只配给「私钥取自配置」的节点；无此类节点则不取值
	if pre.anyNodeUsesConfigKey && cfg.Credential.KeyPassword != "" {
		plain, problems, fatalErr := cache.password(cfg.Credential.KeyPassword, encrypted, true)
		if fatalErr != nil {
			return nil, fatalErr
		}
		errs = append(errs, prefixProblems("密钥口令", problems)...)
		if len(problems) == 0 {
			pre.globalPassphrase = plain
			for i := range pre.rows {
				if pre.rows[i].keyFromConfig {
					pre.rows[i].keyPassRaw = plain
				}
			}
		}
	}

	if len(errs) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "CSV凭据预检查失败，共 %d 处错误：\n", len(errs))
		for _, e := range errs {
			fmt.Fprintf(&b, "  %s\n", e)
		}
		b.WriteString("请修复上述问题后重新执行")
		return nil, errors.New(b.String())
	}
	return pre, nil
}

// unusedKeyNotice 清单里配了私钥、本次却没加 -k 时的提示；没有这种情况返回空串。
//
// 清单里的每一格都是用户当场写下的判断，配了没用上值得提醒一句；
// 配置里的默认值不在此列——那是长期备着的东西，配了没用上不是错误。
// 同一类提示一次运行只出一次：清单可能上千行都配了私钥，逐行提醒没有意义。
func (p *precheckResult) unusedKeyNotice() string {
	rows := p.unusedKeyRows
	if len(rows) == 0 {
		return ""
	}
	const shown = 5
	parts := make([]string, 0, shown)
	for i, r := range rows {
		if i >= shown {
			break
		}
		parts = append(parts, fmt.Sprintf("%d", r))
	}
	list := strings.Join(parts, "、")
	tail := "这几台会用密码登录"
	switch {
	case len(rows) > shown:
		list = fmt.Sprintf("共 %d 行配了私钥（如第 %s 行）", len(rows), list)
	case len(rows) == 1:
		list = fmt.Sprintf("第 %s 行配了私钥", list)
		tail = "这一台会用密码登录"
	default:
		list = fmt.Sprintf("第 %s 行配了私钥", list)
	}
	return fmt.Sprintf("[提示] 清单%s，本次没加 -k，%s\n", list, tail)
}

// credentialValueSpec 把一个凭据位置的取值解释成读取口径：
// 打开加密时它是文件的路径（相对路径拼 secret_dir），不加密时它就是密码 / 口令本身。
//
// 第二个返回值表示「写的是相对名、但 secret_dir 没配」——这是**配置缺项**，不是这一行的
// 写法问题，所以调用方要按行报出去、并把两条去路都给出来（配上 secret_dir，或那一格改写
// 本机绝对路径）。以前这里把这种情形悄悄降级成「按原样找」，于是报出来的是一句
// 「密码文件不存在」，看不出该去配哪儿。
func credentialValueSpec(raw string, encrypted bool, secretDir string) (string, bool) {
	if !encrypted {
		return strings.TrimSpace(raw), false
	}
	p, err := credential.ResolveSecretPath(raw, secretDir)
	if err != nil {
		return "", true
	}
	return p, false
}

// relativeColumnError 第 4/6 列写了相对名、但配置里没有 secret_dir 时那一行的话。
// 行号与列名由 rowPrefix 给出；这里把 secret_dir、配置文件段落与两条去路一次说清。
func relativeColumnError(idx int, ip, kind, raw string) string {
	return fmt.Sprintf("%s写的是相对路径，但配置里没配 credential.secret_dir：%s（提示：在 %s 的 [credential] 段配上 secret_dir，或这一格改写本机绝对路径）",
		rowPrefix(idx, ip, kind), raw, cli.ConfigShown)
}

// ---- 一次运行内的凭据读取去重 ----
//
// 清单里成千上万行往往共用同一个凭据值（最常见的写法：整个清单只填一个密码）。
// 逐行读盘解密既浪费（N 次读盘 + N 次解密），又会在内存里留下 N 份相同明文。
// 这里按「取值 + 读取口径」记住首次读取结果，命中即复用，不再碰磁盘。

// credCache 凭据读取结果表（键含读取口径：加密与否与是否判空都会影响结果）。
type credCache struct {
	entries map[string]credEntry
}

type credEntry struct {
	plain    string
	problems []string
	codes    []credential.CredError // 私钥那支按码分支，见 keyPEM
}

func newCredCache() *credCache {
	return &credCache{entries: map[string]credEntry{}}
}

// password 密码 / 口令类凭据（取值 + 校验，带缓存）。
func (c *credCache) password(value string, encrypted bool, requireNonempty bool) (string, []string, error) {
	key := fmt.Sprintf("pass|%t|%t|%s", encrypted, requireNonempty, value)
	if e, ok := c.entries[key]; ok {
		return e.plain, e.problems, nil
	}
	plain, problems, fatalErr := credential.ReadCredential(value, encrypted, requireNonempty)
	if fatalErr != nil {
		return "", nil, fatalErr // 致命错误来自主密钥而非这个值，原样上抛、不入表
	}
	c.entries[key] = credEntry{plain: plain, problems: problems}
	return plain, problems, nil
}

// keyPEM 私钥 PEM 文件（读盘 + 校验，带缓存）。交出**错误码**而不是文案：
// 调用方要按「找不到」这一种补去路，比不得文案。
func (c *credCache) keyPEM(path string) (string, []credential.CredError) {
	key := "pem|" + path
	if e, ok := c.entries[key]; ok {
		return e.plain, e.codes
	}
	content, codes := credential.ReadCredentialPEMProblems(path)
	c.entries[key] = credEntry{plain: content, codes: codes}
	return content, codes
}

// rowPrefix 凭据问题文案的清单行前缀：行号 + IP + 列名（如「行 3 (IP: 1.2.3.4): 密码文件」）。
func rowPrefix(idx int, ip, kind string) string {
	return fmt.Sprintf("行 %d (IP: %s): %s文件", idx+1, ip, kind)
}

// prefixProblems 给凭据读取返回的问题文案套上调用方前缀
// （清单行用 rowPrefix，默认密码 / 配置口令等无行号场景直接用固定前缀）。
func prefixProblems(prefix string, problems []string) []string {
	if len(problems) == 0 {
		return nil
	}
	out := make([]string, 0, len(problems))
	for _, p := range problems {
		out = append(out, prefix+p)
	}
	return out
}

// padRow 短行补空到 6 列。
func padRow(row []string) []string {
	out := make([]string, 6)
	copy(out, row)
	return out
}
