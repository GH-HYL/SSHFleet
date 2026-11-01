package parse

import (
	"regexp"
	"strings"
	"testing"

	"sshfleet2/internal/message"
)

// 帮助整屏的期望串：从老工程二进制跑 `-h` 采来（管道下宽度 0，即不折行那一档），
// 程序名归一成 PROG、版本行归一成 vTEST。配置取的是随包那份的四格（sudo=开、10/60/300）。
// 排版在宽度 0 这一档逐字相同，加上折行算法本身被字面量断言钉住（message 包），
// 两件事合起来盖住「终端里折行那一档也一致」。
const goldenHelp = `SSHFleet - 批量 SSH 运维工具（命令/脚本执行、文件上传下载）
版本: vTEST

用法:
  PROG -c | -s | -u | -d | --change-password 之一，配合 -f 批量执行
  PROG --gen-key | --key-status | --convert-secret 密钥管理

选项:

  模式（五选一）
    -c --command                          在多台服务器上执行一条命令
    -s --script                           在多台服务器上执行一个本地脚本
    -u --upload                           把本地文件或目录上传到服务器
    -d --download                         从服务器下载文件或目录到本地
       --change-password                  批量改密：把清单里密码过期的账号改成同一个新密码

  清单与目标路径
    -f --csv-file                         节点清单：CSV 文件路径，或直接写一行节点信息（必填）
    -p --path                             目标路径：上传写服务器目录 / 下载写本机目录（上传、下载必填）

  执行选项
    -n --number           [默认: 全部]    并发数：同时操作几台，不填=全部并行
    -r --remark                           备注，用作历史记录文件夹名（不填自动生成）
       --sudo             [当前配置: 开]  这次以 root 身份执行（需目标节点已配免密 sudo）
       --no-sudo                          这次以登录用户身份执行（配置是开时用它改回来）
       --yes                              跳过所有确认直接执行（自动化用，用它之前先手动跑通一次）
    -a --answer                           代填：看到触发词就自动填内容（一条写成「代填内容,触发词,触发词」），多条写进 CSV 文件再指向它
       --no-shell                         不套工具的外壳、不设环境：命令原样下发；只能和 -c 一起用
    -t --timeout          [默认: 60/300]  命令跑完、文件传完的最长等待（秒）
    -T --connect-timeout  [默认: 10]      连上服务器的最长等待（秒）

  密钥与凭据
    -k --key                              密钥登录：不写=只用密码；写了=用清单或配置里的私钥

  密钥管理（不执行批量任务）
       --gen-key                          生成主密钥（打开加密之后才会用到），密钥存进环境变量 SSHFLEET_KEY
       --key-status                       查看主密钥状态：读到哪把、已存哪把、是否一致
       --convert-secret                   转换凭据文件（后面跟要转的文件路径）：按配置里的加密开关，转成明文或密文

示例:
  执行命令:   PROG -f nodes.csv -c "ls -l"
  执行脚本:   PROG -f nodes.csv -s deploy.sh
  代填执行:   PROG -f nodes.csv -s deploy.sh -a "y,是否继续"
  批量改密:   PROG -f nodes.csv --change-password "新密码"
  上传文件:   PROG -f nodes.csv -u ./dist/app -p /opt/app/
  下载文件:   PROG -f nodes.csv -d /var/log/app -p ./logs/
  用密钥登录: PROG -f nodes.csv -c "uptime" -k
  不确认执行: PROG -f nodes.csv -c "uptime" --yes

更多用法、配置说明、示例：见 docs/
`

// 整屏逐字：解析站的事实（选项表 + 配置四格）加 message 的排版，合出来必须与老版一字不差。
func TestHelpScreenMatchesLegacy(t *testing.T) {
	t.Chdir("../..") // 读随包那份 config/SSHFleet.conf
	raw, err := readRawConfig()
	if err != nil {
		t.Fatalf("读随包配置失败：%v", err)
	}
	f, err := executionOf(raw)
	if err != nil {
		t.Fatalf("随包配置四格取不到：%v", err)
	}
	got := helpScreenText("PROG", "TEST", f)
	if got != goldenHelp {
		t.Errorf("帮助整屏与老版不一致：\n--- got ---\n%s\n--- want ---\n%s", got, goldenHelp)
	}
}

// 示例与用法两行里的选项名必须都在选项表里——两处各写一份迟早漂移，这条断言钉住它。
func TestUsageAndExamplesMatchOptionTable(t *testing.T) {
	t.Chdir("../..")
	raw, _ := readRawConfig()
	f, err := executionOf(raw)
	if err != nil {
		t.Fatalf("随包配置四格取不到：%v", err)
	}
	screen := helpScreenText("PROG", "TEST", f)

	table := map[string]bool{}
	for _, o := range options {
		table[o.short] = true
		table[o.long] = true
	}
	re := regexp.MustCompile(`-{1,2}[a-z][a-z0-9-]*`)
	for _, line := range strings.Split(screen, "\n") {
		if !strings.HasPrefix(line, "  PROG") { // 只查用法两行与示例八行
			continue
		}
		// 先剥掉引号段：示例里 `ls -l` 的 -l 是那条命令自己的选项，不是本工具的
		bare := regexp.MustCompile(`"[^"]*"`).ReplaceAllString(line, `""`)
		for _, tok := range re.FindAllString(bare, -1) {
			if !table[tok] {
				t.Errorf("屏上出现选项表里没有的选项名：%q（%s）", tok, line)
			}
		}
	}
}

// helpScreenText 与 helpScreen 只差终端宽度这一格：测试传 0（与采集老版时的管道同条件）。
func helpScreenText(name, num string, f execution) string {
	rev, tm, dirty := buildInfo()
	ver := versionString(num, rev, tm, dirty)
	return message.HelpScreen(name, ver, 0, helpEntries(f)).S
}
