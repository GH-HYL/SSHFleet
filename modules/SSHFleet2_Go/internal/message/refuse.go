package message

// 闸门那几类拒绝的话：闸门只交事实（哪一条没过、哪个选项、哪段原文），句子在这里成形。
//
// 判据是 ADR-0034 决定八（出判断的地方不写话）与 ADR-0028 决定一、五（标签与新行只许在
// 构造点加）。每个函数返回的都是**套好外壳、分好行**的正文，闸门拿到就交给 `Fatal`，一个字
// 都不再添。
//
// 文案逐字照老工程：`internal/cli/check.go` 的 CheckArguments 与 `internal/cli/args.go` 的
// 解析失败那两句。这里是使用者看得见的面，改一个字都是改行为。

// 两句外壳，对位老版 main.go 的两处 wrapFatal。
const (
	headParse     = "选项/参数解析失败"
	headArguments = "选项/参数合规性检查未通过"
)

// shell 给正文套上老版那层壳：`<外壳>：<正文>`。
func shell(head, body string) string { return head + "：" + body }

// RefuseUnknownOption 表里没有这个选项（name 带着横杠，例如 `--zzz`）。
func RefuseUnknownOption(name string) string {
	return shell(headParse, "不认识的选项："+name+"（使用 -h 查看帮助）")
}

// RefuseMissingValue 带值的选项后面没有值（name 带着横杠，例如 `-c`）。
func RefuseMissingValue(name string) string {
	return shell(headParse, "选项 "+name+" 缺少值（使用 -h 查看帮助）")
}

// 三个数值选项在报错里怎么称呼自己。解析站挑一个填进事实（contract.Problem.Detail），
// 句子仍由这里成形——称呼是话，不归站。
const (
	SubjectConcurrency = "并发连接数"
	SubjectTimeout     = "命令或传输超时时间"
	SubjectConnect     = "连接超时时间"
)

// RefuseBadPositiveInt 三个数值选项（-n／-t／-T）取值认不出或不是正数。
// subject 是这一格怎么称呼自己（上面那三个常量之一）。
func RefuseBadPositiveInt(flag, subject, raw string) string {
	return shell(headArguments, ProblemHint(
		flag+" 参数格式错误，"+subject+"必须是正整数，当前值："+raw,
		"给一个正整数（单位见该选项的说明）"))
}

// RefuseModeNone 一个模式都没给。老版把「零个」与「多个」分开说：零个要做的是选一个，
// 不是删掉多余的。
func RefuseModeNone() string {
	return shell(headArguments, ProblemHint(
		"执行模式选项：-c、-s、-u、-d、--change-password 互斥，只能指定一个",
		"你没有指定任何模式，请从下面五个里选一个：\n"+
			"      -c 执行命令   -s 执行脚本   -u 上传文件   -d 下载文件   --change-password 批量改密"))
}

// RefuseModeMultiple 同时给了多个模式。
func RefuseModeMultiple() string {
	return shell(headArguments, ProblemHint(
		"执行模式选项：-c、-s、-u、-d、--change-password 互斥，只能指定一个",
		"一次只能做一件事，请只保留一个模式"))
}

// RefuseNoShellWithOtherMode `--no-shell` 只和 `-c` 一起用。
func RefuseNoShellWithOtherMode() string {
	return shell(headArguments, ProblemHint(
		"--no-shell 只能和 -c 一起使用",
		"请去掉 --no-shell，或改用 -c 执行"))
}

// RefusePasswdWithAnswer 改密不能配 -a。
func RefusePasswdWithAnswer() string {
	return shell(headArguments, ProblemHint(
		"--change-password 不能和 -a 一起用",
		"改密不用你配触发词，用的是一份统一的提示词表"))
}

// RefusePasswdWithPath 改密不能配 -p。
func RefusePasswdWithPath() string {
	return shell(headArguments, ProblemHint(
		"--change-password 不能和 -p 一起用",
		"改密不传文件，用不到目标路径"))
}

// RefusePasswdWithKey 改密不能配 -k。
func RefusePasswdWithKey() string {
	return shell(headArguments, ProblemHint(
		"--change-password 不能和 -k 一起用",
		"改密要用账号自己的登录密码登进去，密钥登录派不上用场"))
}

// RefuseRemotePathNotAbsolute 远端路径（上传目录、下载源）不以 / 开头。
func RefuseRemotePathNotAbsolute(flag, what, value string) string {
	return shell(headArguments, ProblemHint(
		flag+" 参数指定的"+what+"必须是绝对路径，当前值："+value,
		"服务器上的路径要从 / 写起，例如 /opt/app/"))
}

// RefuseRemotePathHasNewline 远端路径里有回车或换行：参数被换行打断的现场。
func RefuseRemotePathHasNewline(flag, what, value string) string {
	return shell(headArguments, ProblemHint(
		flag+" 参数指定的"+what+"里有回车或换行："+value,
		"一条命令只写一个路径，检查复制时是不是把换行带进来了"))
}

// RefuseUploadDirNeedsSlash 上传的 -p 必须以 / 收尾（ADR-0002 的消歧义约束）。
func RefuseUploadDirNeedsSlash(value string) string {
	return shell(headArguments, ProblemHint(
		"上传模式：-p 必须是以 / 结尾的目录，当前值："+value,
		"-p 只表示\"放到哪个目录\"，工具不会替你改文件名，所以要以 / 收尾"))
}

// RefusePathWithCommand `-p` 不能和 `-c`／`-s` 一起用。
func RefusePathWithCommand() string {
	return shell(headArguments, ProblemHint(
		"-p 参数不能搭配 -c 或 -s 使用",
		"只有上传、下载才用 -p；命令与脚本模式不需要它"))
}

// RefuseDownloadNeedsPath `-d` 必须配 `-p`。
func RefuseDownloadNeedsPath() string {
	return shell(headArguments, ProblemHint(
		"-d 参数必须搭配 -p 参数使用",
		"-p 写文件要存到本机的哪个目录，例如 -p ./logs/"))
}

// RefuseUploadNeedsPath `-u` 必须配 `-p`。
func RefuseUploadNeedsPath() string {
	return shell(headArguments, ProblemHint(
		"-u 参数必须搭配 -p 参数使用",
		"-p 写文件要放到服务器上的哪个目录，例如 -p /opt/app/"))
}

// RefuseSudoConflict 两个执行身份开关同给：自相矛盾，只给一行、不补提示（老版如此）。
func RefuseSudoConflict() string {
	return shell(headArguments, "--sudo 与 --no-sudo 只能给一个（两个都不给=按配置里的值）")
}

// RefuseCommandEmpty `-c` 给了但只有空白。
func RefuseCommandEmpty() string {
	return shell(headArguments, ProblemHint(
		"-c 参数不能为空，请提供要执行的命令",
		"-c 后面跟命令原文，例如 -c \"systemctl status sshd\""))
}

// RefusePasswdEmpty `--change-password` 给了但只有空白。
func RefusePasswdEmpty() string {
	return shell(headArguments, ProblemHint(
		"--change-password 的新密码不能为空",
		"后面跟这次要统一改成的新密码"))
}
