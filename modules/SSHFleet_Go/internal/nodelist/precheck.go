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
// 与旧实现的一致性要点：
//   - 状态3(universal)：-k 统一检查一次，忽略节点自带密钥/口令；口令交互输入
//   - 状态2(default)：逐节点检查 CSV 第5列 / 配置默认 account.key 及第6列口令
//   - 状态1(off)：不做密钥预检，强制空密钥，CSV 第5/6列整体忽略（spec D41）
func precheckCredentials(rows [][]string, args *cli.Args, cfg *config.Config, in *common.Interactor) (*precheckResult, error) {
	keyMode := args.KeyMode()
	level := cfg.Account.PasswordSecurity

	pre := &precheckResult{rows: make([]rowCreds, len(rows))}
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
			plain, credErrs, fatalErr := credential.ReadCredential(ppath, level, true)
			if fatalErr != nil {
				return nil, fatalErr
			}
			if len(credErrs) > 0 {
				for _, ce := range credErrs {
					errs = append(errs, fmt.Sprintf("行 %d (IP: %s): 密码文件%s", idx+1, row[0], credential.CredErrorLabel(ce.Code, ppath, ce.Detail)))
				}
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
			keyPlain, credErrs := credential.ReadCredentialPEM(kpath)
			if len(credErrs) > 0 {
				for _, ce := range credErrs {
					errs = append(errs, fmt.Sprintf("行 %d (IP: %s): 密钥文件%s", idx+1, row[0], credential.CredErrorLabel(ce.Code, kpath, ce.Detail)))
				}
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
					plain, credErrs, fatalErr := credential.ReadCredential(ppPath, level, false)
					if fatalErr != nil {
						return nil, fatalErr
					}
					for _, ce := range credErrs {
						errs = append(errs, fmt.Sprintf("行 %d (IP: %s): 私钥口令文件%s", idx+1, row[0], credential.CredErrorLabel(ce.Code, ppPath, ce.Detail)))
					}
					if len(credErrs) == 0 {
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
			plain, credErrs, fatalErr := credential.ReadCredential(cfg.Account.Password, level, true)
			if fatalErr != nil {
				return nil, fatalErr
			}
			for _, ce := range credErrs {
				errs = append(errs, fmt.Sprintf("默认密码文件%s", credential.CredErrorLabel(ce.Code, cfg.Account.Password, ce.Detail)))
			}
			if len(credErrs) == 0 {
				pre.defaultPasswordPlain = plain
			}
		}
	}

	// 配置私钥口令（spec D42：只配给「私钥取自配置」的节点；无此类节点则不读）
	if keyMode == cli.KeyModeDefault && pre.anyNodeUsesConfigKey && cfg.Account.KeyPassphrase != "" {
		plain, credErrs, fatalErr := credential.ReadCredential(cfg.Account.KeyPassphrase, level, true)
		if fatalErr != nil {
			return nil, fatalErr
		}
		for _, ce := range credErrs {
			errs = append(errs, fmt.Sprintf("密钥passphrase文件%s", credential.CredErrorLabel(ce.Code, cfg.Account.KeyPassphrase, ce.Detail)))
		}
		if len(credErrs) == 0 {
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
