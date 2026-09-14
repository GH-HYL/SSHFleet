package credential

import (
	"fmt"
	"os"
	"strings"

	"sshfleet/internal/common"
)

// 主密钥管理（对位旧 master_key.py）：
//   - 运行期从环境变量 SSHFLEET_KEY 读取（spec D15 统一名）
//   - 持久化按平台分流，见 masterkey_windows.go（注册表直读 + setx 写入）与
//     masterkey_unix.go（登录 shell 的 rc 文件，zsh → ~/.zshrc、bash/其他 → ~/.bashrc，spec D30）

const envName = "SSHFLEET_KEY"

// GetMasterKey 从环境变量读取主密钥；缺失时返回带生成指引的错误。
func GetMasterKey() (string, error) {
	key := strings.TrimSpace(os.Getenv(envName))
	if key == "" {
		return "", fmt.Errorf("缺少主密钥，无法解密/加密凭据文件\n请先生成主密钥：SSHFleet --gen-key")
	}
	return key, nil
}

// GenKey 处理 --gen-key：生成随机主密钥并持久化；已有密钥时先确认覆盖，
// 非交互模式拒绝自动覆盖（覆盖后旧密钥加密的文件无法解密）。
func GenKey(in *common.Interactor) error {
	newKey, err := GenerateMasterKey()
	if err != nil {
		return err
	}
	envKey := strings.TrimSpace(os.Getenv(envName))
	persistedKey := readPersistedKey()

	overwritten := false
	if envKey != "" || persistedKey != "" {
		fmt.Fprintln(in.Out, "检测到已存在主密钥，当前未做任何修改")
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
	fmt.Fprint(in.Out, out.String())
	return nil
}
