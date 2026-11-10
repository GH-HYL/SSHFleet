package keytool

import (
	"errors"
	"os"
	"strings"

	"sshfleet2/internal/ask"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/credential"
	"sshfleet2/internal/message"
)

// 三条密钥命令的流程：读现场、问那一句、写、报收场。
//
// 它不判「该不该跑」（那是塔尖的事）、不读命令行（命令行只有解析站读）、不自己拼字
// （话一律向 message 要）、不碰终端（打字走 ask）。主密钥的读写与加解密全在凭据包。
//
// 唯一自己读的东西是配置里的加密开关那一格——三条命令是独立功能，不掺和主流程，
// 要什么自己读（ADR-0036 决定三）。代价认下来：同一份文件有了第二个读取器。

// configPath 与配置本体同一个落点：密钥命令只认这一处相对路径。
//
// 解析站那边也写着同一串（它读整份配置）。两边不许互相 import——密钥包不引用别站，
// 所以认下这两份（开发者 2026-11-06 定）：**改这里要一起改那边**。
const configPath = "./config/SSHFleet.conf"

// Run 跑一条密钥命令，按它交回的结局收场（nil 表示跑完了）。
//
// `upstream` 是主密钥来源：塔尖递进来的就是运行期那一个（`credential.DefaultKeySource`）。
// 显式传而不是在包里取默认值，是为了让用例注入假件——跑测试不许写坏本机的 rc 文件与注册表。
func Run(run contract.KeyRun, upstream *credential.KeySource) *contract.CannotStart {
	switch run.Which {
	case contract.KeyToolGen:
		return genKey(run, upstream)
	case contract.KeyToolStatus:
		ask.Show(message.KeyStatusReport(upstream.Inspect()))
	case contract.KeyToolConvert:
		return convertSecret(run, upstream)
	}
	return nil
}

// genKey 是 `--gen-key`：生成、问要不要覆盖、写持久值、报收尾。
func genKey(run contract.KeyRun, upstream *credential.KeySource) *contract.CannotStart {
	key, err := credential.GenerateMasterKey()
	if err != nil {
		return keyReason(contract.CannotStartReason{Kind: contract.CannotStartKeyGenFailed, Origin: err.Error()})
	}
	facts := upstream.Inspect()

	overwritten := false
	if facts.EnvSet || facts.Persisted {
		ask.Show(message.KeyGenExisting(facts))
		if run.Yes {
			// 非交互模式下不替人决定：覆盖是不可逆的，缺省必须是不覆盖。
			return keyReason(contract.CannotStartReason{Kind: contract.CannotStartKeyGenNoOverwrite})
		}
		if ask.AskQuestion(message.Line{}, message.KeyGenConfirm(), false) != ask.Yes {
			// 答否与读到输入结束是同一个收场：原密钥留着，正常退出（不是取消）。
			ask.Show(message.KeyGenKept())
			return nil
		}
		overwritten = true
		ask.Show(message.KeyGenOverwrite())
	}

	res, err := upstream.Write(key)
	if err != nil {
		return keyReason(contract.CannotStartReason{
			Kind:   contract.CannotStartKeyGenSaveFailed,
			Key:    facts,
			Place:  res.Where,
			Origin: reasonOf(err),
		})
	}
	ask.Show(message.KeyGenSaved(overwritten, facts, res, credential.Fingerprint(key)))
	return nil
}

// convertSecret 是 `--convert-secret`：按加密开关定方向，在明文与密文之间转一次。
func convertSecret(run contract.KeyRun, upstream *credential.KeySource) *contract.CannotStart {
	content, problem, reason := credential.ReadConvertible(run.Path)
	if problem != "" {
		return keyReason(contract.CannotStartReason{
			Kind:   contract.CannotStartKeyConvertReadProblem,
			Place:  run.Path,
			Value:  string(problem),
			Origin: reason,
		})
	}
	encrypted, err := encryptFlag()
	if err != nil {
		return keyReason(contract.CannotStartReason{Kind: contract.CannotStartKeyConfigUnreadable, Origin: reasonOf(err)})
	}
	if encrypted {
		return toCipher(run.Path, content, upstream)
	}
	return toPlain(run.Path, content, upstream)
}

// toCipher 是明文 → 密文。已经是密文就不动；两处主密钥不一致时拦下（加密不可逆）。
func toCipher(path, content string, upstream *credential.KeySource) *contract.CannotStart {
	if credential.IsEncrypted(content) {
		ask.Show(message.KeyConvertAlreadyCipher(path))
		return nil
	}
	facts := upstream.Inspect()
	if facts.State == contract.KeyStateDiverged {
		return keyReason(contract.CannotStartReason{Kind: contract.CannotStartKeyConvertBlocked, Key: facts})
	}
	key, err := credential.MasterKey()
	if err != nil {
		return keyReason(contract.CannotStartReason{Kind: contract.CannotStartKeyNoMasterKey, Key: facts})
	}
	token, err := credential.Encrypt(content, key)
	if err != nil {
		return keyReason(contract.CannotStartReason{Kind: contract.CannotStartKeyEncryptFailed, Origin: err.Error()})
	}
	return writeAndEcho(path, token, "明文", "密文")
}

// toPlain 是密文 → 明文。本来就是明文就不动；解不开、或解出来是空串，都不写回。
func toPlain(path, content string, upstream *credential.KeySource) *contract.CannotStart {
	if !credential.IsEncrypted(content) {
		ask.Show(message.KeyConvertAlreadyPlain(path))
		return nil
	}
	key, err := credential.MasterKey()
	if err != nil {
		return keyReason(contract.CannotStartReason{
		Kind: contract.CannotStartKeyNoMasterKey,
		Key:  upstream.Inspect(),
	})
	}
	plain, err := credential.Decrypt(content, key)
	if err != nil {
		return keyReason(contract.CannotStartReason{
			Kind:   contract.CannotStartKeyConvertDecryptFail,
			Place:  path,
			Origin: reasonOf(err),
		})
	}
	if plain == "" {
		return keyReason(contract.CannotStartReason{Kind: contract.CannotStartKeyConvertEmpty, Place: path})
	}
	return writeAndEcho(path, plain, "密文", "明文")
}

// writeAndEcho 就地写回，再从磁盘回读一次：只报方向、路径与长度，不回显内容本体。
func writeAndEcho(path, newContent, from, to string) *contract.CannotStart {
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return keyReason(contract.CannotStartReason{
			Kind:   contract.CannotStartKeyConvertWriteFail,
			Place:  path,
			Origin: reasonOf(err),
		})
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		return keyReason(contract.CannotStartReason{
			Kind:   contract.CannotStartKeyConvertVerifyFail,
			Place:  path,
			Origin: reasonOf(err),
		})
	}
	ask.Show(message.KeyConvertDone(from, to, path, len(strings.TrimSpace(string(onDisk)))))
	return nil
}

// encryptFlag 读配置里 `[credential] encrypt` 那一格。
//
// 手写扫一遍就够：只认一格布尔，配置文件在解析站那边本来也是手写解析的（没有 TOML 库）。
// 这条路径实际上走不到——进密钥分支之前，解析站已经把整份配置读通一遍。
func encryptFlag() (bool, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false, err
	}
	section := ""
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if section != "credential" {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != "encrypt" {
			continue
		}
		return strings.Trim(stripComment(value), "\"'") == "true", nil
	}
	return false, errors.New(message.ConfigNoEncrypt)
}

// stripComment 去掉行内注释：`encrypt = true   # …` 里那一截。
//
// 这一格只可能是布尔，不必管引号里的 `#`。
func stripComment(value string) string {
	value = strings.TrimSpace(value)
	if i := strings.Index(value, "#"); i >= 0 {
		value = value[:i]
	}
	return strings.TrimSpace(value)
}

// keyReason 排一屏「起不来」：环节名沿用老版的 `credential`，原因按类别与事实交上去，
// 句子由 `message` 成形（ADR-0044）。
//
// 那个环节名不是定位线索，只是逐字一致的一部分：同一屏上还有「无法生成主密钥」这类中文句子，
// 维护者靠那句话找地方，不靠 tag。
func keyReason(reason contract.CannotStartReason) *contract.CannotStart {
	return &contract.CannotStart{Where: "credential", Reason: reason}
}

// reasonOf 取系统那句原因，去掉「open <路径>:」这类前缀（口径同 message.SystemReason）。
func reasonOf(err error) string {
	return credential.SystemReason(err)
}
