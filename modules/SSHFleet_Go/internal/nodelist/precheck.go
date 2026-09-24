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
//   - 第 6 列永不检查：工具无法判定私钥是否加密，报了就是误报，只能靠用户自觉
//
// 同一凭据在一次预检内只读一次：清单多行常共用同一个密码文件，逐行读会重复读盘解密 N 次，
// 并在内存里留下 N 份相同明文；命中缓存的行直接复用首次结果。
func precheckCredentials(rows [][]string, args *cli.Args, cfg *config.Config) (*precheckResult, error) {
	encrypted := cfg.Account.Encrypt

	pre := &precheckResult{rows: make([]rowCreds, len(rows))}
	cache := newCredCache()
	var errs []string

	for idx, raw := range rows {
		row := padRow(raw)
		rc := rowCreds{}

		passwordRaw := strings.TrimSpace(row[3])
		keyRaw := strings.TrimSpace(row[4])

		// 私钥：写了 -k 才用密钥登录，路径取「清单第 5 列 > 配置 account.key」
		key := ""
		if args.Key {
			key = keyRaw
			if key == "" {
				key = cfg.Account.Key
				rc.keyFromConfig = key != ""
			}
			if key == "" {
				errs = append(errs, fmt.Sprintf("行 %d (IP: %s): 加了 -k 却没有私钥可用——清单第 5 列为空，配置 account.key 也没配", idx+1, row[0]))
				pre.rows[idx] = rc
				continue
			}
		}
		rc.hasKey = key != ""

		// 密码列：取值 + 校验
		if passwordRaw != "" {
			plain, problems, fatalErr := cache.password(credentialValueSpec(passwordRaw, encrypted, cfg.Account.SecretDir), encrypted, true)
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
			kpath, rerr := resolveCredentialPath(key, cfg.Account.SecretDir)
			if rerr != nil {
				return nil, rerr
			}
			keyPlain, problems := cache.keyPEM(kpath)
			if len(problems) > 0 {
				errs = append(errs, prefixProblems(rowPrefix(idx, row[0], "密钥"), problems)...)
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
					plain, _, fatalErr := cache.password(credentialValueSpec(passRaw, encrypted, cfg.Account.SecretDir), encrypted, false)
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
	if pre.anyNodeUsesConfigKey && cfg.Account.KeyPassword != "" {
		plain, problems, fatalErr := cache.password(cfg.Account.KeyPassword, encrypted, true)
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

// credentialValueSpec 把一个凭据位置的取值解释成读取口径：
// 打开加密时它是文件的路径（相对路径拼 secret_dir），不加密时它就是密码 / 口令本身。
func credentialValueSpec(raw string, encrypted bool, secretDir string) string {
	if !encrypted {
		return strings.TrimSpace(raw)
	}
	p, err := resolveCredentialPath(raw, secretDir)
	if err != nil {
		return strings.TrimSpace(raw) // 拼不了就原样交给读取层，由它按路径不存在报错
	}
	return p
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

// keyPEM 私钥 PEM 文件（读盘 + 校验，带缓存）。
func (c *credCache) keyPEM(path string) (string, []string) {
	key := "pem|" + path
	if e, ok := c.entries[key]; ok {
		return e.plain, e.problems
	}
	content, problems := credential.ReadCredentialPEM(path)
	c.entries[key] = credEntry{plain: content, problems: problems}
	return content, problems
}

// resolveCredentialPath 清单凭据列路径解析：「相对路径拼 secret_dir」这条规则
// 只对清单生效（配置里的路径一律写全），secret_dir 未配置时报错给出路。
func resolveCredentialPath(raw, secretDir string) (string, error) {
	resolved, err := credential.ResolveSecretPath(raw, secretDir)
	if err != nil {
		if errors.Is(err, credential.ErrRelativeNoSecretDir) {
			return "", fmt.Errorf("凭据列包含相对路径，但 secret_dir 未配置")
		}
		return "", err
	}
	return resolved, nil
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
