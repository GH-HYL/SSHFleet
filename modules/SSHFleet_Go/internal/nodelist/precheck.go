package nodelist

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"sshfleet/internal/cli"
	"sshfleet/internal/common"
	"sshfleet/internal/config"
	"sshfleet/internal/credential"
)

// precheckCredentials 凭据预检（受 -k 三态控制，对位旧 validate_csv_credentials + 读取合并）：
// 校验 + 解码一次完成（读 → 校验 → 直接用），解码值随结果返回。
// 全部通过才进入逐节点解析——保证交互提示不会发生在凭据错误暴露之前。
//
// 同一凭据文件在一次预检内只读一次：清单多行常共用同一个密码文件（甚至整个清单只写一个路径），
// 逐行读会重复读盘解密 N 次，并在内存里留下 N 份相同明文；命中缓存的行直接复用首次结果。
//
// 与旧实现的一致性要点：
//   - 状态3(universal)：-k 统一检查一次，忽略节点自带密钥/口令；口令交互输入
//   - 状态2(default)：逐节点检查 CSV 第5列 / 配置默认 account.key 及第6列口令
//   - 状态1(off)：不做密钥预检，强制空密钥，CSV 第5/6列整体忽略（spec D41）
func precheckCredentials(rows [][]string, args *cli.Args, cfg *config.Config, in *common.Interactor) (*precheckResult, error) {
	keyMode := args.KeyMode()
	level := cfg.Account.PasswordSecurity

	pre := &precheckResult{rows: make([]rowCreds, len(rows))}
	cache := newCredCache()
	var errs []string

	// 状态3：统一检查命令行私钥一次（不走 secret_dir，走当前工作目录）
	if keyMode == cli.KeyModeUniversal {
		if errs2 := checkUniversalKey(args.Key); errs2 != nil {
			errs = append(errs, errs2...)
		}
	}
	for idx, raw := range rows {
		row := padRow(raw)
		rc := rowCreds{}

		passwordRaw := strings.TrimSpace(row[3])
		keyRaw := strings.TrimSpace(row[4])

		// 密钥路径按三态决定（用户 2026-09-11 裁定）：
		//   off（不写 -k）：强制空密钥，清单第 5/6 列整体忽略
		//   default（裸 -k）：清单第 5 列 > 配置默认 account.key
		//   universal（-k 带路径）：强制使用参数指定的私钥，忽略节点自带密钥/口令
		// 私钥与口令**成对绑定**（spec D42）：口令只从私钥的同源位置取，不跨来源混用。
		key := ""
		switch keyMode {
		case cli.KeyModeUniversal:
			key = args.Key
		case cli.KeyModeDefault:
			key = keyRaw
			if key == "" {
				key = cfg.Account.Key
				rc.keyFromConfig = key != ""
			}
		}
		rc.hasKey = key != ""

		// 密码列：解析并验证 + 解码
		if passwordRaw != "" {
			ppath, rerr := resolveCredentialPath(passwordRaw, cfg.Account.SecretDir)
			if rerr != nil {
				return nil, rerr
			}
			plain, problems, fatalErr := cache.password(ppath, level, true)
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

		// 密钥内容读取：
		//   default：PEM 校验 + 内容捕获
		//   universal：内容在循环后统一读取
		//   off：强制空密钥，不读任何密钥文件（用户 2026-09-11 裁定）
		if keyMode == cli.KeyModeDefault && key != "" {
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

			// 第6列：口令只与「清单私钥」成对；私钥取自配置时第6列不参与（spec D42）
			if !rc.keyFromConfig {
				passphraseRaw := strings.TrimSpace(row[5])
				if passphraseRaw != "" {
					ppPath, rerr := resolveCredentialPath(passphraseRaw, cfg.Account.SecretDir)
					if rerr != nil {
						return nil, rerr
					}
					plain, problems, fatalErr := cache.password(ppPath, level, false)
					if fatalErr != nil {
						return nil, fatalErr
					}
					errs = append(errs, prefixProblems(rowPrefix(idx, row[0], "私钥口令"), problems)...)
					if len(problems) == 0 {
						rc.keyPassRaw = plain
					}
				}
			}
		}
		pre.rows[idx] = rc
	}

	// 有空密码行：验证一次默认密码（同时完成解码）
	if pre.needDefaultPassword {
		if cfg.Account.Password == "" {
			errs = append(errs, "密码列有空值，但 config 未配置默认密码(account.password)")
		} else {
			plain, problems, fatalErr := cache.password(cfg.Account.Password, level, true)
			if fatalErr != nil {
				return nil, fatalErr
			}
			errs = append(errs, prefixProblems("默认密码文件", problems)...)
			if len(problems) == 0 {
				pre.defaultPasswordPlain = plain
			}
		}
	}

	// 配置私钥口令（spec D42：只配给「私钥取自配置」的节点；无此类节点则不读）
	if keyMode == cli.KeyModeDefault && pre.anyNodeUsesConfigKey && cfg.Account.KeyPassphrase != "" {
		plain, problems, fatalErr := cache.password(cfg.Account.KeyPassphrase, level, true)
		if fatalErr != nil {
			return nil, fatalErr
		}
		errs = append(errs, prefixProblems("密钥passphrase文件", problems)...)
		if len(problems) == 0 {
			pre.globalPassphrase = plain
			for i := range pre.rows {
				if pre.rows[i].keyFromConfig {
					pre.rows[i].keyPassRaw = plain
				}
			}
		}
	}

	// 状态3：统一私钥内容读取一次；口令交互输入（非交互明确报错，照旧）
	if keyMode == cli.KeyModeUniversal {
		keyContent, err := credential.ReadPEMRaw(expandTilde(args.Key))
		if err != nil {
			errs = append(errs, fmt.Sprintf("-k 指定的私钥文件读取失败 → %s (%v)", args.Key, err))
		} else {
			pre.universalKeyContent = keyContent
		}
		if in.Disinteractive {
			return nil, fmt.Errorf("状态3(-k 路径)需交互输入私钥口令，但处于 --disinteractive 模式；请去掉 --disinteractive 或改用状态2(-k 不带路径)")
		}
		passphrase, perr := in.PromptPassword("请输入私钥口令(无口令直接回车): ")
		if perr != nil {
			return nil, perr
		}
		pre.universalPassphrase = passphrase
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

// ---- 一次运行内的凭据读取去重 ----
//
// 清单里成千上万行往往共用同一个凭据文件（最常见的写法：整个清单只填一个密码文件路径）。
// 逐行读盘解密既浪费（N 次读盘 + N 次解密），又会在内存里留下 N 份相同明文。
// 这里按「解析后的路径 + 读取口径」记住首次读取结果，命中即复用，不再碰磁盘。

// credCache 凭据读取结果表（键含读取口径：等级与是否判空都会影响结果）。
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

// password 密码 / 口令类凭据（读盘 + 校验 + 解码，带缓存）。
func (c *credCache) password(path string, level int, requireNonempty bool) (string, []string, error) {
	key := fmt.Sprintf("pass|%d|%t|%s", level, requireNonempty, path)
	if e, ok := c.entries[key]; ok {
		return e.plain, e.problems, nil
	}
	plain, problems, fatalErr := credential.ReadCredential(path, level, requireNonempty)
	if fatalErr != nil {
		return "", nil, fatalErr // 致命错误来自主密钥而非这个文件，原样上抛、不入表
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

// checkUniversalKey 状态3 的 -k 私钥文件前置检查（存在 / 可读 / 非空 / PEM）。
func checkUniversalKey(kpath string) []string {
	kpath = expandTilde(kpath)
	if _, err := os.Stat(kpath); err != nil {
		return []string{fmt.Sprintf("-k 指定的私钥文件不存在 → %s", kpath)}
	}
	data, err := os.ReadFile(kpath)
	if err != nil {
		return []string{fmt.Sprintf("-k 指定的私钥文件不可读 → %s (%v)", kpath, err)}
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return []string{fmt.Sprintf("-k 指定的私钥文件内容为空 → %s", kpath)}
	}
	if !strings.HasPrefix(content, "-----BEGIN") {
		return []string{fmt.Sprintf("-k 指定的文件不是有效的PEM私钥 → %s", kpath)}
	}
	return nil
}

// resolveCredentialPath CSV 凭据列路径解析：走 D31 梯子单点，
// 「相对路径但 secret_dir 未配置」按 CSV 场景文案报错（对位旧 resolve_credential_path）。
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

func expandTilde(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home2, _ := os.UserHomeDir()
		return home2 + string(p[1]) + p[2:]
	}
	return p
}
