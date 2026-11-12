package parse

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/credential"
	"sshfleet2/internal/criterion"
	"sshfleet2/internal/mask"
)

// 逐台补全：清单那一行 → 一台能连的机器。
//
// 两遍，次序照老版：
//  1. **凭据预检**：先把每行的密码／口令按加密开关解出来。所有行的凭据一次校验完，
//     有一处不对就整批不往下走——凭据错了，后面补出来的端口、用户名都没有意义。
//  2. **逐行补全**：IP、端口、用户名、密码按「清单优先、配置兜底」补齐，毛病一次收齐再报。
//
// 私钥只带走路径，正文与口令到执行站认证那一步再读（那里才用得上 SSH 库）；
// 老版在清单这一步就验 PEM，两版的差别记在 06 号单的 Comments 里。

// nodeColumns 清单的列语义：1 IP、2 端口、3 用户名、4 密码、5 私钥、6 私钥口令。
const nodeColumns = 6

// rowCred 一行预检出来的凭据。
//
// 配置里那两个默认值也放在这里：它们与逐行解出来的值走同一条路（加密开着时要先解），
// 放在同一格里，逐行补全时不必再判断「这个值解过没有」。
type rowCred struct {
	password        string
	keyPass         string
	defaultPassword string
	configKeyPass   string
}

// buildNodes 把清单补成一台台的机器。返回节点、要打给使用者的提示、以及「起不来」。
func buildNodes(rows [][]string, cfg *settings, useKey bool) ([]contract.Node, []contract.Notice, *contract.CannotStart) {
	creds, fail := precheckCredentials(rows, cfg)
	if fail != nil {
		return nil, nil, fail
	}

	var (
		nodes  []contract.Node
		bad    []contract.CannotStartRow
		unused []int
	)
	for idx, raw := range rows {
		row := padRow(raw)
		lineNo := idx + 1
		ip := strings.TrimSpace(row[0])

		var rowErrs []contract.CannotStartReason
		if ip == "" {
			rowErrs = append(rowErrs, contract.CannotStartReason{Kind: contract.CannotStartRowIPMissing})
		} else if !criterion.IsIPv4(ip) {
			rowErrs = append(rowErrs, contract.CannotStartReason{Kind: contract.CannotStartRowIPBad})
		}

		port, portBad := resolvePort(strings.TrimSpace(row[1]), cfg.account.Port)
		if portBad {
			rowErrs = append(rowErrs, contract.CannotStartReason{
				Kind:  contract.CannotStartRowPortBad,
				Value: strings.TrimSpace(row[1]),
			})
		}

		user, userBad := resolveUser(strings.TrimSpace(row[2]), cfg.account.User)
		if userBad {
			rowErrs = append(rowErrs, contract.CannotStartReason{Kind: contract.CannotStartRowUserMissing})
		}

		// 私钥：写了 -k 才用密钥登录，路径取「清单第 5 列 > 配置 credential.key」。
		keyPath := ""
		if useKey {
			keyPath = strings.TrimSpace(row[4])
			if keyPath == "" {
				keyPath = cfg.credentialCfg.Key
			}
			if keyPath == "" {
				rowErrs = append(rowErrs, contract.CannotStartReason{Kind: contract.CannotStartRowKeyMissing})
			}
		} else if strings.TrimSpace(row[4]) != "" {
			unused = append(unused, lineNo)
		}

		// 密码：清单第 4 列解出来的 > 私钥节点留空 > 配置默认解出来的。
		password := creds[idx].password
		if password == "" && keyPath == "" {
			if cfg.account.Password == "" {
				rowErrs = append(rowErrs, contract.CannotStartReason{Kind: contract.CannotStartRowPasswordMissing})
			} else {
				password = creds[idx].defaultPassword
			}
		}

		// 私钥口令只与**清单里那个私钥**成对；私钥取自配置时它不参与。
		keyPass := ""
		if keyPath != "" {
			if keyPath == strings.TrimSpace(row[4]) {
				keyPass = creds[idx].keyPass
			} else {
				keyPass = creds[idx].configKeyPass
			}
		}

		if len(rowErrs) > 0 {
			bad = append(bad, contract.CannotStartRow{Line: lineNo, IP: ip, Items: rowErrs})
			continue
		}

		nodes = append(nodes, contract.Node{
			IP:          ip,
			Port:        strconv.Itoa(port),
			User:        user,
			Password:    password,
			Key:         keyPath,
			KeyPassword: keyPass,
		})
	}

	var notices []contract.Notice
	if len(unused) > 0 {
		notices = append(notices, contract.Notice{Kind: contract.NoticeUnusedKey, Rows: unused})
	}
	if len(bad) > 0 {
		return nil, nil, cannotStartReason("nodelist", contract.CannotStartReason{
			Kind: contract.CannotStartNodeRows,
			Rows: bad,
		})
	}
	return nodes, notices, nil
}

// precheckCredentials 第一遍：把每一行与配置里的密码／口令解成明文，一处不对就整批停。
//
// 报错格式照老版：先一句「CSV凭据预检查失败，共 N 处错误：」，再一行一处。
// 回显的值像路径就原样给（排查要看的就是它），不像路径的遮起来——加密刚打开时清单常
// 还是明文密码，而这段字要进终端与工具日志。
func precheckCredentials(rows [][]string, cfg *settings) ([]rowCred, *contract.CannotStart) {
	creds := make([]rowCred, len(rows))

	defaultPassword, defErr := resolveSecret(cfg.account.Password, cfg)
	configKeyPass, confErr := resolveSecret(cfg.credentialCfg.KeyPassword, cfg)

	var bad []contract.CannotStartRow
	for idx, raw := range rows {
		row := padRow(raw)
		lineNo := idx + 1
		creds[idx].defaultPassword = defaultPassword
		creds[idx].configKeyPass = configKeyPass

		if rawPass := strings.TrimSpace(row[3]); rawPass != "" {
			plain, err := resolveSecret(rawPass, cfg)
			if err != nil {
				bad = append(bad, credRow(lineNo, row[0], "密码", rawPass, err))
				continue
			}
			creds[idx].password = plain
		}
		if rawPass := strings.TrimSpace(row[5]); rawPass != "" {
			plain, err := resolveSecret(rawPass, cfg)
			if err != nil {
				bad = append(bad, credRow(lineNo, row[0], "口令", rawPass, err))
				continue
			}
			creds[idx].keyPass = plain
		}
	}

	// 配置里的默认密码只在有人真的用它时才报错（清单每行都写了密码就不必管它）。
	if needsDefault(rows) && defErr != nil {
		bad = append(bad, configRow(contract.CannotStartRowCredConfigPassword, defErr))
	}
	if confErr != nil {
		bad = append(bad, configRow(contract.CannotStartRowCredConfigKeyPassword, confErr))
	}

	if len(bad) > 0 {
		return nil, cannotStartReason("nodelist", contract.CannotStartReason{
			Kind: contract.CannotStartCredRows,
			Rows: bad,
		})
	}
	return creds, nil
}

// needsDefault 有没有哪一行要靠配置里的默认密码（清单没写密码、也没有私钥）。
func needsDefault(rows [][]string) bool {
	for _, raw := range rows {
		row := padRow(raw)
		if strings.TrimSpace(row[3]) == "" && strings.TrimSpace(row[4]) == "" {
			return true
		}
	}
	return false
}

// credRow 凭据预检的一行：`行 N (IP: x): 密码文件<哪一处不对> → <回显> (<系统原文>)`。
//
// 三种不对分开说——它们的下一步不是一件事：文件不在、解不开、读不了。回显像路径就原样给
// （排查要看的就是它），不像路径的遮起来：加密刚打开时清单常还是明文密码，而这段字要进
// 终端与工具日志。
func credRow(lineNo int, ip, field, raw string, err error) contract.CannotStartRow {
	shown := raw
	if !strings.ContainsAny(raw, `/\`) {
		shown = mask.Secret(raw)
	}
	item := contract.CannotStartReason{
		Kind:  credKind(err),
		Place: field,
		Value: shown,
	}
	// 系统那句原因照原样带出来：只说「不存在」会把「没权限」也说成不存在（ADR-0025 决定 6）。
	var pe *fs.PathError
	if errors.As(err, &pe) {
		item.Origin = pe.Err.Error()
	}
	return contract.CannotStartRow{Line: lineNo, IP: ip, Items: []contract.CannotStartReason{item}}
}

// configRow 是配置级的那两条：不挂在清单某一行上，所以在屏上没有行号前缀。
func configRow(kind contract.CannotStartKind, err error) contract.CannotStartRow {
	return contract.CannotStartRow{Items: []contract.CannotStartReason{{Kind: kind, Origin: err.Error()}}}
}

// credKind 把凭据包的失败归到三条里的一条。
//
// 老版把凭据失败分成一串码，这里只留这一阶段真会撞上的三种：文件不在、读不了、
// 解不开（含主密钥没读到——它和解不开是同一个下一步：先去看主密钥）。
func credKind(err error) contract.CannotStartKind {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return contract.CannotStartRowCredMissing
	case errors.Is(err, credential.ErrKeyMissing), errors.Is(err, credential.ErrBadCipher):
		return contract.CannotStartRowCredBadCipher
	default:
		return contract.CannotStartRowCredUnreadable
	}
}

// resolveSecret 把一个凭据位置解释成明文。
//
// 加密关着：那一格就是密码本身，原样交回（不修剪空白——本程序不替使用者改凭据）。
// 加密开着：那一格是密文文件的路径；相对名先拼 `secret_dir`（只服务清单第 4、6 列与
// 配置里的凭据字段，别处不用它），再交给凭据包解。
func resolveSecret(raw string, cfg *settings) (string, error) {
	if !cfg.credentialCfg.Encrypt {
		return raw, nil
	}
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		if cfg.credentialCfg.SecretDir == "" {
			return "", errNoSecretDir
		}
		path = filepath.Join(cfg.credentialCfg.SecretDir, path)
	}
	return credential.Secret(path, true)
}

// errNoSecretDir 写的是相对名、配置里却没有 secret_dir：这是配置缺项，不是这一行的写法问题。
var errNoSecretDir = errors.New("写的是相对路径，但配置里没配 credential.secret_dir（提示：在配置的 [credential] 段配上 secret_dir，或这一格改写本机绝对路径）")

// padRow 把短行列补到六列：缺的列按空值处理，由各自的补全规则去兜底。
func padRow(row []string) []string {
	out := make([]string, nodeColumns)
	copy(out, row)
	return out
}

// resolvePort 端口补全：清单第 2 列 > 配置 account.port。
//
// 只认纯数字、1-65535；配置侧的范围由读配置那一步保证。
func resolvePort(raw string, defaultPort int) (int, bool) {
	if raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || !allDigits(raw) || v < 1 || v > 65535 {
			return 0, true
		}
		return v, false
	}
	return defaultPort, false
}

// allDigits 纯数字判定：`+22`、` 22` 都不算。
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// resolveUser 用户名补全：清单第 3 列 > 配置 account.user。
func resolveUser(raw, defaultUser string) (string, bool) {
	if raw != "" {
		return raw, false
	}
	if defaultUser != "" {
		return defaultUser, false
	}
	return "", true
}
