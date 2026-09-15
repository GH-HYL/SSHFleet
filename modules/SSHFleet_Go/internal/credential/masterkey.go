package credential

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"

	"sshfleet/internal/common"
)

// 主密钥管理（对位旧 master_key.py）：
//   - 运行期从环境变量 SSHFLEET_KEY 读取（spec D15 统一名）
//   - 持久化按平台分流，见 masterkey_windows.go（注册表直读 + setx 写入）与
//     masterkey_unix.go（登录 shell 的 rc 文件，zsh → ~/.zshrc、bash/其他 → ~/.bashrc，spec D30）
//
// 2026-09-15 增补：**来源一致性检测**。密钥有两个来源——进程环境变量（本次运行实际用的）
// 与本机持久值（下次运行会读的）。两者不一致时，用「环境里那把旧密钥」加密出来的文件
// 重开终端后就解不开了；而用户看到的只会是一句含糊的「主密钥不匹配」。故：
//   - 读密钥时若不一致 → 打一次警告，把两处指纹与出路说清（每进程一次，避免逐节点刷屏）
//   - 加密（写等级3 密文）前若不一致 → **直接拦下**（不可逆，不给侥幸空间）

const envName = "SSHFLEET_KEY"

// 提示配色（与其它模块一致的黄/红）。
const (
	colorReset  = "\x1b[0m"
	colorYellow = "\x1b[33m"
)

// KeySources 主密钥的来源视图。
type KeySources struct {
	Env            string // 进程环境变量 SSHFLEET_KEY（本次运行实际使用）
	Persisted      string // 本机持久值（Windows 注册表 / Unix rc 文件）
	PersistedWhere string // 持久值所在位置
	OtherKey       string // 另一处持久值（Unix 的另一侧 rc 文件；Windows 恒为空）
	OtherWhere     string
}

// EnvSet 环境变量是否已设置。
func (s KeySources) EnvSet() bool { return strings.TrimSpace(s.Env) != "" }

// PersistedSet 本机是否已保存主密钥。
func (s KeySources) PersistedSet() bool { return strings.TrimSpace(s.Persisted) != "" }

// Diverged 两处来源不一致（环境 ≠ 持久值，或两侧 rc 文件互相不同）。
func (s KeySources) Diverged() bool {
	if s.EnvSet() && s.PersistedSet() && strings.TrimSpace(s.Env) != strings.TrimSpace(s.Persisted) {
		return true
	}
	if s.PersistedSet() && s.OtherKey != "" && s.OtherKey != s.Persisted {
		return true
	}
	return false
}

// Fingerprint 密钥指纹（SHA-256 前 8 位十六进制）。
// 用于比对两处密钥是否同一把，而不必（也不该）把密钥本身打到终端或日志里。
func Fingerprint(key string) string {
	if strings.TrimSpace(key) == "" {
		return "（未设置）"
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:4])
}

// divergenceNote 不一致时的事实描述（一致则返回空串）。只讲现状与后果，
// 出路由各调用方按场景追加（读密钥时提示重载，加密时给拦截指引）。纯函数，便于断言。
func divergenceNote(s KeySources) string {
	if !s.Diverged() {
		return ""
	}
	var b strings.Builder
	b.WriteString("主密钥来源不一致——这会让凭据解密报「主密钥不匹配」，且用旧的那把加密出的文件重开终端后就解不开\n")
	b.WriteString("  本次运行实际使用：")
	if s.EnvSet() {
		b.WriteString(fmt.Sprintf("指纹 %s（进程环境变量 %s）\n", Fingerprint(s.Env), envName))
	} else {
		b.WriteString(fmt.Sprintf("未设置（进程环境变量 %s 为空）\n", envName))
	}
	if s.PersistedSet() {
		b.WriteString(fmt.Sprintf("  本机已保存：      指纹 %s（%s）\n", Fingerprint(s.Persisted), s.PersistedWhere))
	}
	if s.OtherKey != "" {
		b.WriteString(fmt.Sprintf("  另一处持久值：    指纹 %s（%s）\n", Fingerprint(s.OtherKey), s.OtherWhere))
	}
	return b.String()
}

// KeyDivergenceNote 读当前来源并生成不一致提示（一致时为空串）。
func KeyDivergenceNote() string { return divergenceNote(ReadKeySources()) }

// divergenceWarned 本次运行是否已经就「两处主密钥来源不一致」提醒过。
//
// 这件事挂在逐节点读凭据这条高频路径上（几万个节点就是几万次），同一句话不能刷屏；
// 预检也会说这件事（更完整的版本）。所以两处共用一个「已经说过」的事实，
// 而不是共用一张只能被消费一次的票——谁先开口谁置位，提醒的有无与条数
// 都不再取决于调用顺序。
var (
	divergenceWarnMu sync.Mutex
	divergenceWarned bool
)

// warnKeyDivergenceOnce 提醒两处来源不一致（本次运行至多一次）；已经说过就不再开口。
func warnKeyDivergenceOnce() {
	divergenceWarnMu.Lock()
	defer divergenceWarnMu.Unlock()
	if divergenceWarned {
		return
	}
	divergenceWarned = true
	note := KeyDivergenceNote()
	if note == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "\n%s[警告]%s %s  让两处一致：%s\n",
		colorYellow, colorReset, note, reloadHint())
}

// GetMasterKey 从环境变量读取主密钥；缺失时返回带生成指引的错误。
// 两处来源不一致时打一次警告并继续（解密方向不能拦：文件可能正是用当前这把加的密）。
func GetMasterKey() (string, error) {
	key := strings.TrimSpace(os.Getenv(envName))
	if key == "" {
		// 环境变量为空要分两种情况：本机其实已保存了一把（终端没读到）——此时提示重载即可，
		// 绝不能让用户去重新生成（会换掉密钥、旧密文全废）。
		if note := PrecheckKey(); note != "" {
			return "", fmt.Errorf("缺少主密钥，无法解密/加密凭据文件\n%s", note)
		}
		return "", fmt.Errorf("缺少主密钥，无法解密/加密凭据文件\n请先生成主密钥：SSHFleet --gen-key")
	}
	warnKeyDivergenceOnce()
	return key, nil
}

// GuardEncrypt 加密（写等级 3 密文）前的硬保护：两处密钥不一致时直接拦下。
// 加密方向必须拦——用旧密钥加密出的文件重开终端后就解不开了，属不可逆风险。
func GuardEncrypt() error {
	note := KeyDivergenceNote()
	if note == "" {
		return nil
	}
	return fmt.Errorf("已阻止加密。\n%s"+
		"出路：\n  ① %s，然后重试\n  ② 若确定要用本次运行这把密钥：%s，然后重试",
		note, reloadHint(), persistHint())
}

// ---- 主密钥自检：把「忘了 source / 重开终端」这件事交给工具发现，而不是靠人记住 ----

// KeyState 主密钥来源的组合状态。
type KeyState string

const (
	// KeyStateOK 两处一致，正常可用。
	KeyStateOK KeyState = "ok"
	// KeyStateDiverged 两处都有但不同（改了密钥没重载，或哪个终端里手动 export 过别的）。
	KeyStateDiverged KeyState = "diverged"
	// KeyStateStaleShell 已保存但当前终端读不到（生成后忘了 source / 重开终端）。
	KeyStateStaleShell KeyState = "stale_shell"
	// KeyStateUnpersisted 当前终端有值但没持久化（手动 export，关掉终端即失效）。
	KeyStateUnpersisted KeyState = "unpersisted"
	// KeyStateUnset 两处都空（首次使用）。
	KeyStateUnset KeyState = "unset"
)

// InspectKey 判定当前主密钥状态并返回来源明细。
func InspectKey() (KeyState, KeySources) {
	src := ReadKeySources()
	switch {
	case src.Diverged():
		return KeyStateDiverged, src
	case src.EnvSet() && src.PersistedSet():
		return KeyStateOK, src
	case src.PersistedSet():
		return KeyStateStaleShell, src
	case src.EnvSet():
		return KeyStateUnpersisted, src
	default:
		return KeyStateUnset, src
	}
}

// sourceLines 两处来源的指纹明细（报告与提示共用）。
func sourceLines(s KeySources) string {
	var b strings.Builder
	if s.EnvSet() {
		b.WriteString(fmt.Sprintf("  本次运行实际使用：指纹 %s（进程环境变量 %s）\n", Fingerprint(s.Env), envName))
	} else {
		b.WriteString(fmt.Sprintf("  本次运行实际使用：未读到（进程环境变量 %s 为空）\n", envName))
	}
	if s.PersistedSet() {
		b.WriteString(fmt.Sprintf("  本机已保存：      指纹 %s（%s）\n", Fingerprint(s.Persisted), s.PersistedWhere))
	} else {
		b.WriteString("  本机已保存：      无\n")
	}
	if s.OtherKey != "" {
		b.WriteString(fmt.Sprintf("  另一处持久值：    指纹 %s（%s）\n", Fingerprint(s.OtherKey), s.OtherWhere))
	}
	return b.String()
}

// markDivergenceWarned 标记「这件事已经说过」：预检自己会打一段更完整的提示，
// 稍后逐节点读凭据时不再重复同一件事。
func markDivergenceWarned() {
	divergenceWarnMu.Lock()
	defer divergenceWarnMu.Unlock()
	divergenceWarned = true
}

// PrecheckKey 开工前的主密钥预检查：正常状态返回空串，异常返回一段可直接打印的提示。
// 用户（以及接手的人）最容易忘的就是「生成密钥后 source / 重开终端」，
// 与其让他在用凭据时报一句看不懂的错，不如每次运行都主动说清现状与下一步。
func PrecheckKey() string {
	state, src := InspectKey()
	switch state {
	case KeyStateDiverged:
		markDivergenceWarned()
		return "主密钥两处来源不一致：\n" + sourceLines(src) +
			"  影响：用「本次运行这把」加密的文件，重开终端后就解不开了（加密已被工具拦下）\n" +
			"  让两处一致：" + reloadHint() + "\n"
	case KeyStateStaleShell:
		return "当前终端没读到已保存的主密钥（凭据解密/加密会失败）：\n" +
			fmt.Sprintf("  本机已保存：      指纹 %s（%s）\n", Fingerprint(src.Persisted), src.PersistedWhere) +
			"  不需要重新生成（--gen-key 会换成另一把，旧密文就解不开了），让它生效即可：" + reloadHint() + "\n"
	case KeyStateUnpersisted:
		return "当前主密钥没有持久化（只活在这个终端里）：\n" +
			fmt.Sprintf("  本次运行实际使用：指纹 %s（进程环境变量 %s）\n", Fingerprint(src.Env), envName) +
			"  影响：关掉终端后密钥就没了，用它加密的凭据文件将解不开\n" +
			"  保持这把密钥不变并持久化：" + persistHint() + "\n"
	}
	return ""
}

// KeyStatusReport --key-status 的完整报告（四种状态都给现状 + 下一步）。
func KeyStatusReport() string {
	state, src := InspectKey()
	var b strings.Builder
	switch state {
	case KeyStateOK:
		b.WriteString("主密钥状态：正常（当前终端读到的与已保存的一致）\n")
		b.WriteString(sourceLines(src))
		b.WriteString("  下一步：无。等级 3（加密）凭据可正常读写了。\n")
	case KeyStateDiverged:
		b.WriteString(colorYellow + "主密钥状态：不一致" + colorReset + "\n")
		b.WriteString(sourceLines(src))
		b.WriteString("  影响：用「本次运行这把」加密出的文件，重开终端后就解不开；凭据解密也可能报「主密钥不匹配」\n")
		b.WriteString("  下一步：" + reloadHint() + "，让本次运行改用已保存的那把\n")
		b.WriteString("         若确定要用本次运行这把：" + persistHint() + "\n")
	case KeyStateStaleShell:
		b.WriteString(colorYellow + "主密钥状态：已保存，但当前终端没读到" + colorReset + "\n")
		b.WriteString(sourceLines(src))
		b.WriteString("  原因：生成密钥后忘了让它生效（这就是「改了密钥反而不能用」的典型场景）\n")
		b.WriteString("  下一步：" + reloadHint() + "；本机已有密钥，不需要 --gen-key\n")
	case KeyStateUnpersisted:
		b.WriteString(colorYellow + "主密钥状态：未持久化" + colorReset + "\n")
		b.WriteString(sourceLines(src))
		b.WriteString("  影响：关掉终端后这把密钥就没了，用它加密的凭据文件将解不开\n")
		b.WriteString("  下一步：保持这把密钥不变并持久化——" + persistHint() + "\n")
	case KeyStateUnset:
		b.WriteString("主密钥状态：未设置\n")
		b.WriteString(sourceLines(src))
		b.WriteString("  下一步：先生成主密钥——SSHFleet --gen-key（等级 3 加密凭据需要它）\n")
	}
	return b.String()
}

// GenKey 处理 --gen-key：生成随机主密钥并持久化；已有密钥时先确认覆盖，
// 非交互模式拒绝自动覆盖（覆盖后旧密钥加密的文件无法解密）。
func GenKey(in *common.Interactor) error {
	newKey, err := GenerateMasterKey()
	if err != nil {
		return err
	}
	src := ReadKeySources()

	overwritten := false
	if src.EnvSet() || src.PersistedSet() {
		fmt.Fprintln(in.Out, "检测到已存在主密钥，当前未做任何修改")
		if src.EnvSet() {
			fmt.Fprintf(in.Out, "  本次运行实际使用：指纹 %s（进程环境变量 %s）\n", Fingerprint(src.Env), envName)
		} else {
			fmt.Fprintf(in.Out, "  本次运行实际使用：未设置（进程环境变量 %s 为空）\n", envName)
		}
		if src.PersistedSet() {
			fmt.Fprintf(in.Out, "  本机已保存：      指纹 %s（%s）\n", Fingerprint(src.Persisted), src.PersistedWhere)
		}
		if src.Diverged() {
			fmt.Fprintf(in.Out, "  %s→ 两处不一致%s——这正是凭据解密报「主密钥不匹配」的常见原因\n",
				colorYellow, colorReset)
		}
		fmt.Fprintln(in.Out, "注意：如果覆盖，用旧密钥加密的凭据文件将永久无法解密")
		if in.Disinteractive {
			return fmt.Errorf("检测到已存在主密钥，非交互模式不自动覆盖（覆盖后旧密钥加密的凭据文件将无法解密）。\n如需覆盖：去掉 --disinteractive 后重新执行 --gen-key")
		}
		confirmed, err := in.Confirm("是否确认覆盖？", false)
		if err != nil {
			// 对位旧 _confirm_overwrite：EOF 视为「否」，保留原密钥正常返回（不作为取消）
			fmt.Fprintln(in.Out, "已保留原密钥，未做修改")
			return nil
		}
		if !confirmed {
			fmt.Fprintln(in.Out, "已保留原密钥，未做修改")
			return nil
		}
		overwritten = true
		fmt.Fprintln(in.Out, "已确认覆盖，正在重新生成主密钥...")
	}

	var out strings.Builder
	if err := persistKey(newKey, overwritten, &out); err != nil {
		return err
	}
	fmt.Fprintf(&out, "  新密钥指纹：%s\n", Fingerprint(newKey))
	fmt.Fprint(in.Out, out.String())
	return nil
}
