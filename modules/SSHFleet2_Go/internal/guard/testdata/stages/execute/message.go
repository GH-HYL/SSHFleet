package execute

// 样本：拼报文那一处所在的包。环节名三种来法都在这儿——直传常量、经一层传参、
// 赋给本地变量再拼——检查器该三种都收得着。

const messagePrefix = "sshfleet."

const (
	envRun    = "run"
	envPasswd = "passwd"

	runTimeout     = "timeout"
	runRejected    = "rejected"
	passwdGiveUp   = "give_up"
	passwdNoPrompt = "no_prompt"
)

func msg(env, stage, detail string) string {
	return messagePrefix + env + "." + stage + ": " + detail
}

func timeoutLine() string {
	return msg(envRun, runTimeout, "exceeded")
}

// runFail 环节名由参数进来：只有到调用点才看得见是哪一个。
func runFail(stage string, detail string) string {
	return msg(envRun, stage, detail)
}

func rejectedLine() string {
	return runFail(runRejected, "refused to run")
}

// passwdLine 先赋给本地变量、再拼：两种情形各说一句，所以变量有两种取值。
func passwdLine(canceled bool) string {
	stage := passwdGiveUp
	if canceled {
		stage = passwdNoPrompt
	}
	return msg(envPasswd, stage, "no further prompts")
}
