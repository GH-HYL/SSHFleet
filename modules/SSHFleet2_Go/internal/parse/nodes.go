package parse

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/credential"
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
func buildNodes(rows [][]string, cfg *settings, useKey bool) ([]contract.Node, []contract.Text, *contract.CannotStart) {
	creds, fail := precheckCredentials(rows, cfg)
	if fail != nil {
		return nil, nil, fail
	}

	var (
		nodes  []contract.Node
		errs   []string
		unused []int
	)
	for idx, raw := range rows {
		row := padRow(raw)
		lineNo := idx + 1
		ip := strings.TrimSpace(row[0])

		var rowErrs []string
		if ip == "" {
			rowErrs = append(rowErrs, "IP必须存在")
		} else if !isIPv4(ip) {
			rowErrs = append(rowErrs, "IP格式不正确")
		}

		port, portErrs := resolvePort(strings.TrimSpace(row[1]), cfg.account.Port)
		rowErrs = append(rowErrs, portErrs...)

		user, userErr := resolveUser(strings.TrimSpace(row[2]), cfg.account.User)
		if userErr != nil {
			rowErrs = append(rowErrs, userErr.Error())
		}

		// 私钥：写了 -k 才用密钥登录，路径取「清单第 5 列 > 配置 credential.key」。
		keyPath := ""
		if useKey {
			keyPath = strings.TrimSpace(row[4])
			if keyPath == "" {
				keyPath = cfg.credentialCfg.Key
			}
			if keyPath == "" {
				rowErrs = append(rowErrs, "用了 -k，但没有找到可用的私钥——在第 5 列写私钥路径，或把统一私钥配到配置 credential.key")
			}
		} else if strings.TrimSpace(row[4]) != "" {
			unused = append(unused, lineNo)
		}

		// 密码：清单第 4 列解出来的 > 私钥节点留空 > 配置默认解出来的。
		password := creds[idx].password
		if password == "" && keyPath == "" {
			if cfg.account.Password == "" {
				rowErrs = append(rowErrs, "密码为空：清单第 4 列没写，配置 account.password 也没配")
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
			shown := ip
			if shown == "" {
				shown = "空值"
			}
			errs = append(errs, fmt.Sprintf("行 %d (IP: %s): - %s", lineNo, shown, strings.Join(rowErrs, "，")))
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

	var notices []contract.Text
	if notice := unusedKeyNotice(unused); notice != "" {
		notices = append(notices, contract.Text{S: notice})
	}
	if len(errs) > 0 {
		return nil, nil, cannotStart("nodelist", "", "发现以下错误:"+"\n"+strings.Join(errs, "\n"))
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

	var errs []string
	for idx, raw := range rows {
		row := padRow(raw)
		lineNo := idx + 1
		creds[idx].defaultPassword = defaultPassword
		creds[idx].configKeyPass = configKeyPass

		if rawPass := strings.TrimSpace(row[3]); rawPass != "" {
			plain, err := resolveSecret(rawPass, cfg)
			if err != nil {
				errs = append(errs, rowIssue(lineNo, row[0], "密码", rawPass, err))
				continue
			}
			creds[idx].password = plain
		}
		if rawPass := strings.TrimSpace(row[5]); rawPass != "" {
			plain, err := resolveSecret(rawPass, cfg)
			if err != nil {
				errs = append(errs, rowIssue(lineNo, row[0], "口令", rawPass, err))
				continue
			}
			creds[idx].keyPass = plain
		}
	}

	// 配置里的默认密码只在有人真的用它时才报错（清单每行都写了密码就不必管它）。
	if needsDefault(rows) && defErr != nil {
		errs = append(errs, "配置 account.password："+defErr.Error())
	}
	if confErr != nil {
		errs = append(errs, "配置 credential.key_password："+confErr.Error())
	}

	if len(errs) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "CSV凭据预检查失败，共 %d 处错误：\n", len(errs))
		for _, e := range errs {
			b.WriteString("  " + e + "\n")
		}
		b.WriteString("请修复上述问题后重新执行")
		return nil, cannotStart("nodelist", "", b.String())
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

// rowIssue 凭据那一行的话：`行 N (IP: x): 密码文件<哪一处不对> → <回显> (<系统原文>)`。
//
// 三种不对分开说——它们的下一步不是一件事：文件不在、解不开、读不了。
func rowIssue(lineNo int, ip, kind, raw string, err error) string {
	shown := raw
	if !strings.ContainsAny(raw, `/\`) {
		shown = mask.Secret(raw)
	}
	line := fmt.Sprintf("行 %d (IP: %s): %s文件%s → %s", lineNo, ip, kind, credLabel(err), shown)
	// 系统那句原因照原样带出来：只说「不存在」会把「没权限」也说成不存在（ADR-0025 决定 6）。
	var pe *fs.PathError
	if errors.As(err, &pe) {
		line += " (" + pe.Err.Error() + ")"
	}
	return line
}

// credLabel 把凭据包的失败翻成老版那几句话。
//
// 老版把凭据失败分成一串码，这里只留这一阶段真会撞上的三种：文件不在、读不了、
// 解不开（含主密钥没读到——它和解不开是同一个下一步：先去看主密钥）。
func credLabel(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "不存在（打开加密时这里填的是凭据文件的绝对路径，不加密时才是密码本身）"
	case errors.Is(err, credential.ErrKeyMissing), errors.Is(err, credential.ErrBadCipher):
		return "不是有效的密文，或主密钥不匹配（先用 --convert-secret 重新转换）"
	default:
		return "无法读取"
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

// unusedKeyNotice 是「清单里配了私钥、这次没加 -k」的那句提示。
//
// 只说前五行、多于一行的说「共 N 行」——清单可能几百行，一句提示刷满屏没有意义。
func unusedKeyNotice(rows []int) string {
	if len(rows) == 0 {
		return ""
	}
	const shown = 5
	parts := make([]string, 0, shown)
	for i, r := range rows {
		if i >= shown {
			break
		}
		parts = append(parts, strconv.Itoa(r))
	}
	list := strings.Join(parts, "、")
	tail := "这几台会用密码登录"
	switch {
	case len(rows) > shown:
		list = fmt.Sprintf("共 %d 行配了私钥（如第 %s 行）", len(rows), list)
	case len(rows) == 1:
		list = "第 " + list + " 行配了私钥"
		tail = "这一台会用密码登录"
	default:
		list = "第 " + list + " 行配了私钥"
	}
	return fmt.Sprintf("[提示] 清单%s，本次没加 -k，%s\n", list, tail)
}

// padRow 把短行列补到六列：缺的列按空值处理，由各自的补全规则去兜底。
func padRow(row []string) []string {
	out := make([]string, nodeColumns)
	copy(out, row)
	return out
}

// resolvePort 端口补全：清单第 2 列 > 配置 account.port。
//
// 只认纯数字、1-65535；配置侧的范围由读配置那一步保证。
func resolvePort(raw string, defaultPort int) (int, []string) {
	if raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || !allDigits(raw) || v < 1 || v > 65535 {
			return 0, []string{"端口必须是1-65535之间的整数，当前值为：" + raw}
		}
		return v, nil
	}
	return defaultPort, nil
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
func resolveUser(raw, defaultUser string) (string, error) {
	if raw != "" {
		return raw, nil
	}
	if defaultUser != "" {
		return defaultUser, nil
	}
	return "", fmt.Errorf("用户名为空：清单第 3 列没写，配置 account.user 也没配")
}
